package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"wuzapi/database"

	"github.com/go-resty/resty/v2"
	"github.com/mdp/qrterminal/v3"
	"github.com/patrickmn/go-cache"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store"
	_ "modernc.org/sqlite"

	//	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	//"google.golang.org/protobuf/proto"

	"github.com/go-redis/redis/v8"
)

// var wlog waLog.Logger
var clientPointer = make(map[int]*whatsmeow.Client)
var clientHttp = make(map[int]*resty.Client)
var historySyncID int32
var ctx = context.Background()

type MyClient struct {
	WAClient       *whatsmeow.Client
	eventHandlerID uint32
	userID         int
	token          string
	subscriptions  []string
	db             *sql.DB
	service        database.Service
	instance       string
}

// Connects to Whatsapp Websocket on server startup if last state was connected
func (s *server) connectOnStartup() {

	users, err := s.service.ListConnectedUsers()

	if err != nil {
		log.Error().Err(err).Msg("DB Problem")
		return
	}

	for _, user := range users {
		// log.Info().Str("token", user.Token).Msg("Connect to Whatsapp on startup")

		v := Values{map[string]string{
			"Id":      strconv.Itoa(int(user.ID)),
			"Jid":     user.Jid,
			"Webhook": user.Webhook,
			"Token":   user.Token,
			"Events":  user.Events,
		}}

		userinfocache.Set(user.Token, v, cache.NoExpiration)
		userid, _ := strconv.Atoi(strconv.Itoa(int(user.ID)))
		// Gets and set subscription to webhook events
		eventarray := strings.Split(user.Events, ",")

		var subscribedEvents []string
		if len(eventarray) < 1 {
			if !Find(subscribedEvents, "All") {
				subscribedEvents = append(subscribedEvents, "All")
			}
		} else {
			for _, arg := range eventarray {
				if !Find(messageTypes, arg) {
					log.Warn().Str("Type", arg).Msg("Message type discarded")
					continue
				}
				if !Find(subscribedEvents, arg) {
					subscribedEvents = append(subscribedEvents, arg)
				}
			}
		}
		eventstring := strings.Join(subscribedEvents, ",")
		log.Info().Str("events", eventstring).Str("jid", user.Jid).Msg("Attempt to connect")
		killchannel[userid] = make(chan bool)
		go s.startClient(userid, user.Jid, user.Token, subscribedEvents, false, make(chan bool))
	}

	// checar postgres sintexe
	/* rows, err := s.db.Query("SELECT id,token,jid,webhook,events FROM users WHERE connected=1")
	if err != nil {
		log.Error().Err(err).Msg("DB Problem")
		return
	}
	defer rows.Close()
	for rows.Next() {
		txtid := ""
		token := ""
		jid := ""
		webhook := ""
		events := ""
		err = rows.Scan(&txtid, &token, &jid, &webhook, &events)
		if err != nil {
			log.Error().Err(err).Msg("DB Problem")
			return
		} else {
			log.Info().Str("token", token).Msg("Connect to Whatsapp on startup")
			v := Values{map[string]string{
				"Id":      txtid,
				"Jid":     jid,
				"Webhook": webhook,
				"Token":   token,
				"Events":  events,
			}}
			userinfocache.Set(token, v, cache.NoExpiration)
			userid, _ := strconv.Atoi(txtid)
			// Gets and set subscription to webhook events
			eventarray := strings.Split(events, ",")

			var subscribedEvents []string
			if len(eventarray) < 1 {
				if !Find(subscribedEvents, "All") {
					subscribedEvents = append(subscribedEvents, "All")
				}
			} else {
				for _, arg := range eventarray {
					if !Find(messageTypes, arg) {
						log.Warn().Str("Type", arg).Msg("Message type discarded")
						continue
					}
					if !Find(subscribedEvents, arg) {
						subscribedEvents = append(subscribedEvents, arg)
					}
				}
			}
			eventstring := strings.Join(subscribedEvents, ",")
			log.Info().Str("events", eventstring).Str("jid", jid).Msg("Attempt to connect")
			killchannel[userid] = make(chan bool)
			go s.startClient(userid, jid, token, subscribedEvents)
		}
	}
	err = rows.Err()
	if err != nil {
		log.Error().Err(err).Msg("DB Problem")
	} */
}

func parseJID(arg string) (types.JID, bool) {
	if arg == "" {
		return types.NewJID("", types.DefaultUserServer), false
	}
	if arg[0] == '+' {
		arg = arg[1:]
	}

	// Basic only digit check for recipient phone number, we want to remove @server and .session
	phonenumber := ""
	phonenumber = strings.Split(arg, "@")[0]
	phonenumber = strings.Split(phonenumber, ".")[0]

	// fmt.Println("phonenumber", phonenumber)
	/* 	b := true
	   	for _, c := range phonenumber {
	   		if c < '0' || c > '9' {
	   			b = false
	   			break
	   		}
	   	}
	   	if b == false {
	   		log.Warn().Msg("Bad jid format, return empty")
	   		recipient, _ := types.ParseJID("")
	   		return recipient, false
	   	} */

	if !strings.ContainsRune(arg, '@') {
		return types.NewJID(arg, types.DefaultUserServer), true
	} else {
		recipient, err := types.ParseJID(arg)
		if err != nil {
			log.Error().Err(err).Str("jid", arg).Msg("Invalid jid")
			return recipient, false
		} else if recipient.User == "" {
			log.Error().Err(err).Str("jid", arg).Msg("Invalid jid. No server specified")
			return recipient, false
		}
		return recipient, true
	}
}

func (s *server) startClient(userID int, textjid string, token string, subscriptions []string, pairing bool, done chan bool) {

	log.Info().Str("userid", strconv.Itoa(userID)).Str("jid", textjid).Msg("Starting websocket connection to Whatsapp")
	var deviceStore *store.Device
	var err error
	instance := os.Getenv("INSTANCE")

	if instance == "" {
		log.Error().Msg("INSTANCE is not set")
		return
	}

	if clientPointer[userID] != nil {
		isConnected := clientPointer[userID].IsConnected()
		if isConnected == true && !pairing {
			return
		}
	}

	if textjid != "" {
		jid, _ := parseJID(textjid)
		deviceStore, err = container.GetDevice(jid)
		if err != nil {
			panic(err)
		}
	} else {
		log.Warn().Msg("No jid found. Creating new device")
		deviceStore = container.NewDevice()
	}

	if deviceStore == nil {
		log.Warn().Msg("No store found. Creating new one")
		deviceStore = container.NewDevice()
	}

	osName := "Windows"
	store.DeviceProps.PlatformType = waProto.DeviceProps_CHROME.Enum()
	store.DeviceProps.Os = &osName

	clientLog := waLog.Stdout("Client", *waDebug, true)
	var client *whatsmeow.Client

	if *waDebug != "" {
		client = whatsmeow.NewClient(deviceStore, clientLog)
	} else {
		client = whatsmeow.NewClient(deviceStore, nil)
	}

	clientPointer[userID] = client
	mycli := MyClient{client, 1, userID, token, subscriptions, s.db, s.service, instance}
	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.myEventHandler)
	clientHttp[userID] = resty.New()
	clientHttp[userID].SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))

	if *waDebug == "DEBUG" {
		clientHttp[userID].SetDebug(true)
	}

	clientHttp[userID].SetTimeout(5 * time.Second)
	clientHttp[userID].SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

	if client.Store.ID == nil {
		// No ID stored, new login

		qrChan, err := client.GetQRChannel(ctx)
		if err != nil {
			// This error means that we're already logged in, so ignore it.
			if !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
				log.Error().Err(err).Msg("Failed to get QR channel")
			}
		} else {
			err = client.Connect() // Si no conectamos no se puede generar QR
			if err != nil {
				panic(err)
			}

			for evt := range qrChan {
				log.Info().Str("event", evt.Event).Msg("Login event")
				if evt.Event == "code" {
					// Display QR code in terminal (useful for testing/developing)
					if *logType != "json" {
						qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
						fmt.Println("QR code:\n", evt.Code)
					}
					// Store encoded/embeded base64 QR on database for retrieval with the /qr endpoint
					image, _ := qrcode.Encode(evt.Code, qrcode.Medium, 256)
					base64qrcode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)

					err := s.service.SetQrcode(userID, base64qrcode, instance)
					if err != nil {
						log.Error().Err(err).Msg("Could not update QR code")
					}

					if pairing {
						pairingCode, err := client.PairPhone(textjid, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
						if err != nil {
							log.Error().Err(err).Msg("Failed to pair phone")
						}

						errPar := s.service.SetPairingCode(userID, pairingCode, instance)
						if errPar != nil {
							log.Error().Err(err).Msg("Could not update QR code")
						}
						pairing = false
						log.Info().Str("pairingCode", pairingCode).Msg("Pairing code")
					}

					// Sinalizar que a operação foi concluída
					done <- true
				} else if evt.Event == "timeout" {
					err := s.service.SetQrcode(userID, "", instance)
					if err != nil {
						log.Error().Err(err).Msg("Could not update QR code")
					}

					log.Warn().Msg("QR timeout killing channel")
					delete(clientPointer, userID)
					killchannel[userID] <- true
				} else if evt.Event == "success" {
					log.Info().Msg("QR pairing ok!")
					err := s.service.SetQrcode(userID, "", instance)
					if err != nil {
						log.Error().Err(err).Msg("Could not update QR code")
					}
					// Sinalizar que a operação foi concluída
					done <- true
				}
			}
		}
	} else {
		log.Info().Msg("Already logged in, just connect")
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	for {
		select {
		case <-killchannel[userID]:
			log.Info().Str("userid", strconv.Itoa(userID)).Msg("Received kill signal")
			client.Disconnect()
			delete(clientPointer, userID)
			err := s.service.SetDisconnected(userID)
			if err != nil {
				log.Error().Err(err).Msg("Could not update user as disconnected")
			}
			return
		default:
			time.Sleep(1000 * time.Millisecond)
		}
	}
}

// func (s *server) startClient(userID int, textjid string, token string, subscriptions []string) (string, error) {

// 	log.Info().Str("userid", strconv.Itoa(userID)).Str("jid", textjid).Msg("Starting websocket connection to Whatsapp")
// 	var deviceStore *store.Device
// 	var err error

// 	fmt.Println("clientPointer", clientPointer)
// 	if clientPointer[userID] != nil {
// 		isConnected := clientPointer[userID].IsConnected()
// 		if isConnected == true {
// 			return "", nil
// 		}
// 	}

// 	if textjid != "" && textjid != "PAIRPHONE" {
// 		jid, _ := parseJID(textjid)
// 		deviceStore, err = container.GetDevice(jid)
// 		if err != nil {
// 			return "", err
// 		}
// 	} else {
// 		log.Warn().Msg("No jid found. Creating new device")
// 		deviceStore = container.NewDevice()
// 	}

// 	if deviceStore == nil {
// 		log.Warn().Msg("No store found. Creating new one")
// 		deviceStore = container.NewDevice()
// 	}

// 	osName := "Windows"
// 	store.DeviceProps.PlatformType = waProto.DeviceProps_CHROME.Enum()
// 	store.DeviceProps.Os = &osName

// 	clientLog := waLog.Stdout("Client", *waDebug, true)
// 	var client *whatsmeow.Client

// 	if *waDebug != "" {
// 		client = whatsmeow.NewClient(deviceStore, clientLog)
// 	} else {
// 		client = whatsmeow.NewClient(deviceStore, nil)
// 	}

// 	clientPointer[userID] = client
// 	mycli := MyClient{client, 1, userID, token, subscriptions, s.db, s.service}
// 	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.myEventHandler)
// 	clientHttp[userID] = resty.New()
// 	clientHttp[userID].SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))

// 	if *waDebug == "DEBUG" {
// 		clientHttp[userID].SetDebug(true)
// 	}

// 	clientHttp[userID].SetTimeout(5 * time.Second)
// 	clientHttp[userID].SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

// 	generateQRCode := func() (string, error) {
// 		qrChan, err1 := client.GetQRChannel(context.Background())

// 		if err1 != nil {
// 			if !errors.Is(err1, whatsmeow.ErrQRStoreContainsID) {
// 				log.Error().Err(err1).Msg("Failed to get QR channel")
// 				return "", err1
// 			}
// 			return "", nil
// 		}

// 		err = client.Connect()
// 		if err != nil {
// 			return "", err
// 		}

// 		for evt := range qrChan {
// 			if evt.Event == "code" {
// 				if *logType != "json" {
// 					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
// 					fmt.Println("QR code:\n", evt.Code)
// 				}

// 				image, _ := qrcode.Encode(evt.Code, qrcode.Medium, 256)
// 				base64qrcode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)

// 				err := s.service.SetQrcode(userID, base64qrcode)
// 				if err != nil {
// 					log.Error().Err(err).Msg("Could not update QR code")
// 				}
// 				return base64qrcode, nil
// 			} else if evt.Event == "timeout" {
// 				err := s.service.SetQrcode(userID, "")
// 				if err != nil {
// 					log.Error().Err(err).Msg("Could not update QR code")
// 				}

// 				log.Warn().Msg("QR timeout killing channel")
// 				delete(clientPointer, userID)
// 				killchannel[userID] <- true
// 				break
// 			} else if evt.Event == "success" {
// 				log.Info().Msg("QR pairing ok!")
// 				err := s.service.SetQrcode(userID, "")
// 				if err != nil {
// 					log.Error().Err(err).Msg("Could not update QR code")
// 				}
// 			} else {
// 				log.Info().Str("event", evt.Event).Msg("Login event")
// 			}
// 		}

// 		return "", nil
// 	}

// 	if textjid == "PAIRPHONE" {
// 		return generateQRCode()
// 	}

// 	if client.Store.ID == nil {
// 		base64qrcode, err := generateQRCode()
// 		if err != nil {
// 			return "", err
// 		}
// 		return base64qrcode, nil
// 	}

// 	log.Info().Msg("Already logged in, just connect")
// 	err = client.Connect()
// 	if err != nil {
// 		return "", err
// 	}

// 	for {
// 		select {
// 		case <-killchannel[userID]:
// 			log.Info().Str("userid", strconv.Itoa(userID)).Msg("Received kill signal")
// 			client.Disconnect()
// 			delete(clientPointer, userID)

// 			err := s.service.SetDisconnected(userID)
// 			if err != nil {
// 				log.Error().Err(err).Msg("Could not update user as disconnected")
// 			}

// 			return "", nil
// 		default:
// 			time.Sleep(1000 * time.Millisecond)
// 		}
// 	}
// }

func (mycli *MyClient) myEventHandler(rawEvt interface{}) {
	txtid := strconv.Itoa(mycli.userID)
	postmap := make(map[string]interface{})
	postmap["event"] = rawEvt
	dowebhook := 0
	path := ""
	redisuri := os.Getenv("REDIS_URI")
	redispass := os.Getenv("REDIS_PASS")
	dbname := os.Getenv("DB_NAME")
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	switch evt := rawEvt.(type) {
	case *events.AppStateSyncComplete:
		if len(mycli.WAClient.Store.PushName) > 0 && evt.Name == appstate.WAPatchCriticalBlock {
			err := mycli.WAClient.SendPresence(types.PresenceAvailable)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to send available presence")
			} else {
				log.Info().Msg("Marked self as available")
			}
		}
	case *events.Connected, *events.PushNameSetting:
		log.Info().Msg("Connected event received")
		if len(mycli.WAClient.Store.PushName) == 0 {
			return
		}
		// Send presence available when connecting and when the pushname is changed.
		// This makes sure that outgoing messages always have the right pushname.
		err := mycli.WAClient.SendPresence(types.PresenceAvailable)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to send available presence")
		} else {
			log.Info().Msg("Marked self as available")
		}

		err = mycli.service.SetConnected(mycli.userID)

		/* sqlStmt := `UPDATE users SET connected=1 WHERE id=?`
		_, err = mycli.db.Exec(sqlStmt, mycli.userID) */

		if err != nil {
			log.Error().Err(err).Msg("Could not update user as connected")
			return
		}

	case *events.PairSuccess:
		log.Info().Str("userid", strconv.Itoa(mycli.userID)).Str("token", mycli.token).Str("ID", evt.ID.String()).Str("BusinessName", evt.BusinessName).Str("Platform", evt.Platform).Msg("QR Pair Success")
		jid := evt.ID

		// checar postgres sintexe

		err := mycli.service.SetJid(mycli.userID, jid.String())
		/* sqlStmt := `UPDATE users SET jid=? WHERE id=?`
		_, err := mycli.db.Exec(sqlStmt, jid, mycli.userID) */
		if err != nil {
			log.Error().Err(err).Msg("Could not update jid")
			return
		}

		err = mycli.service.SetConnected(mycli.userID)

		if err != nil {
			log.Error().Err(err).Msg("Could not update user as connected")
			return
		}

		myuserinfo, found := userinfocache.Get(mycli.token)
		if !found {
			log.Warn().Msg("No user info cached on pairing?")
		} else {
			txtid := myuserinfo.(Values).Get("Id")
			token := myuserinfo.(Values).Get("Token")
			v := updateUserInfo(myuserinfo, "Jid", fmt.Sprintf("%s", jid))
			userinfocache.Set(token, v, cache.NoExpiration)
			log.Info().Str("jid", jid.String()).Str("userid", txtid).Str("token", token).Msg("User information set")
		}
	case *events.StreamReplaced:
		log.Info().Msg("Received StreamReplaced event")
		return
	case *events.Message:
		postmap["type"] = "Message"
		dowebhook = 1
		metaParts := []string{fmt.Sprintf("pushname: %s", evt.Info.PushName), fmt.Sprintf("timestamp: %s", evt.Info.Timestamp)}
		if evt.Info.Type != "" {
			metaParts = append(metaParts, fmt.Sprintf("type: %s", evt.Info.Type))
		}
		if evt.Info.Category != "" {
			metaParts = append(metaParts, fmt.Sprintf("category: %s", evt.Info.Category))
		}
		if evt.IsViewOnce {
			metaParts = append(metaParts, "view once")
		}
		if evt.IsViewOnce {
			metaParts = append(metaParts, "ephemeral")
		}

		log.Info().Str("id", evt.Info.ID).Str("source", evt.Info.SourceString()).Str("parts", strings.Join(metaParts, ", ")).Msg("Message Received")

		// try to get Image if any
		img := evt.Message.GetImageMessage()
		if img != nil {

			// check/creates user directory for files
			userDirectory := fmt.Sprintf("%s/files/user_%s", exPath, txtid)
			_, err := os.Stat(userDirectory)
			if os.IsNotExist(err) {
				errDir := os.MkdirAll(userDirectory, 0751)
				if errDir != nil {
					log.Error().Err(errDir).Msg("Could not create user directory")
					return
				}
			}

			data, err := mycli.WAClient.Download(img)
			if err != nil {
				log.Error().Err(err).Msg("Failed to download image")
				return
			}
			exts, _ := mime.ExtensionsByType(img.GetMimetype())
			path = fmt.Sprintf("%s/%s%s", userDirectory, evt.Info.ID, exts[0])
			err = os.WriteFile(path, data, 0600)
			if err != nil {
				log.Error().Err(err).Msg("Failed to save image")
				return
			}
			log.Info().Str("path", path).Msg("Image saved")
		}

		// try to get Audio if any
		audio := evt.Message.GetAudioMessage()
		if audio != nil {

			// check/creates user directory for files
			userDirectory := fmt.Sprintf("%s/files/user_%s", exPath, txtid)
			_, err := os.Stat(userDirectory)
			if os.IsNotExist(err) {
				errDir := os.MkdirAll(userDirectory, 0751)
				if errDir != nil {
					log.Error().Err(errDir).Msg("Could not create user directory")
					return
				}
			}

			data, err := mycli.WAClient.Download(audio)
			if err != nil {
				log.Error().Err(err).Msg("Failed to download audio")
				return
			}
			exts, _ := mime.ExtensionsByType(audio.GetMimetype())
			path = fmt.Sprintf("%s/%s%s", userDirectory, evt.Info.ID, exts[0])
			err = os.WriteFile(path, data, 0600)
			if err != nil {
				log.Error().Err(err).Msg("Failed to save audio")
				return
			}
			log.Info().Str("path", path).Msg("Audio saved")
		}

		// try to get Document if any
		document := evt.Message.GetDocumentMessage()
		if document != nil {

			// check/creates user directory for files
			userDirectory := fmt.Sprintf("%s/files/user_%s", exPath, txtid)
			_, err := os.Stat(userDirectory)
			if os.IsNotExist(err) {
				errDir := os.MkdirAll(userDirectory, 0751)
				if errDir != nil {
					log.Error().Err(errDir).Msg("Could not create user directory")
					return
				}
			}

			data, err := mycli.WAClient.Download(document)
			if err != nil {
				log.Error().Err(err).Msg("Failed to download document")
				return
			}
			extension := ""
			exts, err := mime.ExtensionsByType(document.GetMimetype())
			if err != nil {
				extension = exts[0]
			} else {
				filename := document.FileName
				extension = filepath.Ext(*filename)
			}
			path = fmt.Sprintf("%s/%s%s", userDirectory, evt.Info.ID, extension)
			err = os.WriteFile(path, data, 0600)
			if err != nil {
				log.Error().Err(err).Msg("Failed to save document")
				return
			}
			log.Info().Str("path", path).Msg("Document saved")
		}
	case *events.Receipt:
		postmap["type"] = "ReadReceipt"
		dowebhook = 1
		if evt.Type == events.ReceiptTypeRead || evt.Type == events.ReceiptTypeReadSelf {
			log.Info().Strs("id", evt.MessageIDs).Str("source", evt.SourceString()).Str("timestamp", fmt.Sprintf("%d", evt.Timestamp)).Msg("Message was read")
			if evt.Type == events.ReceiptTypeRead {
				postmap["state"] = "Read"
			} else {
				postmap["state"] = "ReadSelf"
			}
		} else if evt.Type == events.ReceiptTypeDelivered {
			postmap["state"] = "Delivered"
			log.Info().Str("id", evt.MessageIDs[0]).Str("source", evt.SourceString()).Str("timestamp", fmt.Sprintf("%d", evt.Timestamp)).Msg("Message delivered")
		} else {
			// Discard webhooks for inactive or other delivery types
			return
		}
	case *events.Presence:
		postmap["type"] = "Presence"
		dowebhook = 1
		if evt.Unavailable {
			postmap["state"] = "offline"
			if evt.LastSeen.IsZero() {
				log.Info().Str("from", evt.From.String()).Msg("User is now offline")
			} else {
				log.Info().Str("from", evt.From.String()).Str("lastSeen", fmt.Sprintf("%d", evt.LastSeen)).Msg("User is now offline")
			}
		} else {
			postmap["state"] = "online"
			log.Info().Str("from", evt.From.String()).Msg("User is now online")
		}
	case *events.HistorySync:
		postmap["type"] = "HistorySync"
		dowebhook = 1

		// check/creates user directory for files
		userDirectory := fmt.Sprintf("%s/files/user_%s", exPath, txtid)
		_, err := os.Stat(userDirectory)
		if os.IsNotExist(err) {
			errDir := os.MkdirAll(userDirectory, 0751)
			if errDir != nil {
				log.Error().Err(errDir).Msg("Could not create user directory")
				return
			}
		}

		id := atomic.AddInt32(&historySyncID, 1)
		fileName := fmt.Sprintf("%s/history-%d.json", userDirectory, id)
		file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			log.Error().Err(err).Msg("Failed to open file to write history sync")
			return
		}
		enc := json.NewEncoder(file)
		enc.SetIndent("", "  ")
		err = enc.Encode(evt.Data)
		if err != nil {
			log.Error().Err(err).Msg("Failed to write history sync")
			return
		}
		log.Info().Str("filename", fileName).Msg("Wrote history sync")
		_ = file.Close()
	case *events.AppState:
		log.Info().Str("index", fmt.Sprintf("%+v", evt.Index)).Str("actionValue", fmt.Sprintf("%+v", evt.SyncActionValue)).Msg("App state event received")
	case *events.LoggedOut:
		log.Info().Str("reason", evt.Reason.String()).Msg("Logged out")

		err := mycli.service.SetDisconnected(mycli.userID)

		if err != nil {
			log.Error().Err(err).Msg("Could not update user as disconnected")
			return
		}
		killchannel[mycli.userID] <- true

	case *events.ChatPresence:
		postmap["type"] = "ChatPresence"
		dowebhook = 1
		log.Info().Str("state", fmt.Sprintf("%s", evt.State)).Str("media", fmt.Sprintf("%s", evt.Media)).Str("chat", evt.MessageSource.Chat.String()).Str("sender", evt.MessageSource.Sender.String()).Msg("Chat Presence received")
	case *events.CallOffer:
		log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call offer")
	case *events.CallAccept:
		log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call accept")
	case *events.CallTerminate:
		log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call terminate")
	case *events.CallOfferNotice:
		log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call offer notice")
	case *events.CallRelayLatency:
		log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call relay latency")
	default:
		log.Warn().Str("event", fmt.Sprintf("%+v", evt)).Msg("Unhandled event")
	}

	if redisuri != "" {
		values, _ := json.Marshal(postmap)

		data := make(map[string]string)
		data["jsonData"] = string(values)
		data["token"] = mycli.token

		redisdb := redis.NewClient(&redis.Options{
			Addr:     redisuri,
			Password: redispass,
			DB:       0,
		})

		// Testa a conexão
		_, err := redisdb.Ping(ctx).Result()
		if err != nil {
			log.Error().Err(err).Msg("Erro ao conectar ao Redis")
		}

		fmt.Println("Conectado ao Redis")

		// Nome da fila
		queueName := fmt.Sprintf("bull:%s-Whatsmeow-Messages", dbname)

		// Adiciona mensagens na fila
		err1 := addToQueue(redisdb, queueName, data)
		if err1 != nil {
			log.Error().Err(err1).Msg("Erro ao adicionar mensagem à fila")
		}

		fmt.Println("Mensagens adicionadas à fila")
	}

	if dowebhook == 1 {
		// call webhook
		webhookurl := ""
		myuserinfo, found := userinfocache.Get(mycli.token)
		if !found {
			log.Warn().Str("token", mycli.token).Msg("Could not call webhook as there is no user for this token")
		} else {
			webhookurl = myuserinfo.(Values).Get("Webhook")
		}

		if !Find(mycli.subscriptions, postmap["type"].(string)) && !Find(mycli.subscriptions, "All") {
			log.Warn().Str("type", postmap["type"].(string)).Msg("Skipping webhook. Not subscribed for this type")
			return
		}

		if webhookurl != "" {
			log.Info().Str("url", webhookurl).Msg("Calling webhook")
			values, _ := json.Marshal(postmap)
			if path == "" {
				data := make(map[string]string)
				data["jsonData"] = string(values)
				data["token"] = mycli.token
				go callHook(webhookurl, data, mycli.userID)
			} else {
				data := make(map[string]string)
				data["jsonData"] = string(values)
				data["token"] = mycli.token
				go callHookFile(webhookurl, data, mycli.userID, path)
			}
		}
		// } else {
		// 	values, _ := json.Marshal(postmap)

		// 	data := make(map[string]string)
		// 	data["jsonData"] = string(values)
		// 	data["token"] = mycli.token

		// 	fmt.Println("MENSAGEM", data)
		// 	log.Warn().Str("userid", strconv.Itoa(mycli.userID)).Msg("No webhook set for user")
		// }
	}
}

func addToQueue(client *redis.Client, queueName string, message map[string]string) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	// Cria a chave com o nome da fila
	queueKey := "bull:" + queueName

	// Adiciona a mensagem à fila dentro da chave da fila
	if err := client.RPush(ctx, queueKey, data).Err(); err != nil {
		return err
	}

	return nil
}
