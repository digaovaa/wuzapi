package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	"wuzapi/database"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

type server struct {
	service database.Service
	db      *sql.DB
	router  *mux.Router
	exPath  string
}

var (
	address    = flag.String("address", "0.0.0.0", "Bind IP Address")
	port       = flag.String("port", "8080", "Listen Port")
	waDebug    = flag.String("wadebug", "", "Enable whatsmeow debug (INFO or DEBUG)")
	logType    = flag.String("logtype", "console", "Type of log output (console or json)")
	sslcert    = flag.String("sslcertificate", "", "SSL Certificate File")
	sslprivkey = flag.String("sslprivatekey", "", "SSL Certificate Private Key File")
	container  *sqlstore.Container

	killchannel   = make(map[int](chan bool))
	userinfocache = cache.New(1*time.Minute, 2*time.Minute)
	log           zerolog.Logger
)

func init() {
	flag.Parse()

	if *logType == "json" {
		log = zerolog.New(os.Stdout).With().Timestamp().Str("role", filepath.Base(os.Args[0])).Logger()
	} else {
		output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
		log = zerolog.New(output).With().Timestamp().Str("role", filepath.Base(os.Args[0])).Logger()
	}
}

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal().Err(err).Msg("Error loading .env file")
		panic("Error loading .env file")
	}

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}

	this_dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	fmt.Println(this_dir)
	exPath := filepath.Dir(ex)

	dbDirectory := exPath + "/dbdata"
	fmt.Println(dbDirectory)
	_, err = os.Stat(dbDirectory)
	if os.IsNotExist(err) {
		errDir := os.MkdirAll(dbDirectory, 0751)
		if errDir != nil {
			fmt.Printf("%q: %s\n", err, "Could not create dbdata directory")
			panic("Could not create dbdata directory")
		}
	}

	driver := os.Getenv("DB_DRIVER")
	fmt.Println("Driver: " + driver)

	service, connString, err := database.NewService(exPath, driver)
	if err != nil {
		log.Fatal().Err(err).Msg("Error starting database")
		panic("Error starting database")
	}

	if driver == "sqlite" {
		connString = "file:" + connString + "?_pragma=foreign_keys(1)&_busy_timeout=3000"
	}

	dbLog := waLog.Stdout("Database", "WARN", true)
	container, err = sqlstore.New(driver, connString, dbLog)
	if err != nil {
		panic(err)
	}
	container.Upgrade()

	s := &server{
		router:  mux.NewRouter(),
		exPath:  exPath,
		service: service,
	}

	s.routes()

	s.connectOnStartup()

	srv := &http.Server{
		Addr:    *address + ":" + *port,
		Handler: s.router,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if *sslcert != "" {
			if err := srv.ListenAndServeTLS(*sslcert, *sslprivkey); err != nil && err != http.ErrServerClosed {
				log.Fatal().Err(err).Msg("Startup failed")
			}
		} else {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal().Err(err).Msg("Startup failed")
			}
		}
	}()
	// log.Info().Str("address", *address).Str("port", *port).Msg("Server Started")

	<-done
	// log.Info().Msg("Server Stopped")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
		if container != nil {
			container.Close()
		}
		if s.db != nil {
			s.db.Close()
		}
	}()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Str("error", fmt.Sprintf("%+v", err)).Msg("Server Shutdown Failed")
		os.Exit(1)
	}
	// log.Info().Msg("Server Exited Properly")
}
