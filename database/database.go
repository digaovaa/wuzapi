package database

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/rs/zerolog/log"
)

var (
	dbInstance *gorm.DB
	dbConnStr  string
	dbOnce     sync.Once
)

type Service interface {
	CreateUser(user *User) (int, error)
	UpdateUser(user *User) error
	DeleteUser(id int) error
	SetQrcode(id int, qrcode string, instance string) error
	SetWebhook(id int, webhook string) error
	SetConnected(id int) error
	SetDisconnected(id int) error
	SetJid(id int, jid string) error
	SetEvents(id int, events string) error
	GetUserById(id int) (*User, error)
	GetUserByToken(token string) (*User, error)
	// ListConnectedUsers retorna todos os usuários conectados
	ListConnectedUsers() ([]*User, error)
	// SetPairingCode salva o código de pairing do usuário
	SetPairingCode(id int, pairingCode string, instance string) error
	// SetCountMsg incrementa o contador de mensagens diárias do usuário
	SetCountMsg(id uint, typeMsg string) error
}

type User struct {
	gorm.Model
	ID               uint   `gorm:"primaryKey"`
	Name             string `gorm:"type:text;not null"`
	Token            string `gorm:"type:text;not null"`
	Webhook          string `gorm:"type:text;not null;default:''"`
	Jid              string `gorm:"type:text;not null;default:''"`
	Qrcode           string `gorm:"type:text;not null;default:''"`
	Connected        int    `gorm:"type:integer"`
	Expiration       int    `gorm:"type:integer"`
	Events           string `gorm:"type:text;not null;default:'All'"`
	PairingCode      string `gorm:"type:text;not null;default:''"`
	Instance         string `gorm:"type:text;not null;default:''"`
	CountTextMsg     int    `gorm:"type:integer;default:0"`
	CountImageMsg    int    `gorm:"type:integer;default:0"`
	CountVoiceMsg    int    `gorm:"type:integer;default:0"`
	CountVideoMsg    int    `gorm:"type:integer;default:0"`
	CountStickerMsg  int    `gorm:"type:integer;default:0"`
	CountLocationMsg int    `gorm:"type:integer;default:0"`
	CountContactMsg  int    `gorm:"type:integer;default:0"`
	CountDocumentMsg int    `gorm:"type:integer;default:0"`
}

type UserHistory struct {
	gorm.Model
	ID               uint      `gorm:"primaryKey"`
	UserID           uint      `gorm:"not null"`
	User             *User     `gorm:"foreignKey:UserID"`
	Date             time.Time `gorm:"type:timestamp;not null"`
	CountTextMsg     int       `gorm:"type:integer;default:0"`
	CountImageMsg    int       `gorm:"type:integer;default:0"`
	CountVoiceMsg    int       `gorm:"type:integer;default:0"`
	CountVideoMsg    int       `gorm:"type:integer;default:0"`
	CountStickerMsg  int       `gorm:"type:integer;default:0"`
	CountLocationMsg int       `gorm:"type:integer;default:0"`
	CountContactMsg  int       `gorm:"type:integer;default:0"`
	CountDocumentMsg int       `gorm:"type:integer;default:0"`
	IsOnline         bool      `gorm:"type:boolean;default:false"`
}

type service struct {
	db *gorm.DB
}

func startMysql() (*gorm.DB, string, error) {
	// log.Info().Msg("Starting mysql")

	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", dbUser, dbPass, dbHost, dbPort, dbName)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})

	if err != nil {
		log.Fatal().Err(err).Msg("Could not open/create " + dsn)
		return nil, "", err
	}

	return db, dsn, nil
}

// startPostgres initializes the database connection if it hasn't been already
func startPostgres() (*gorm.DB, string, error) {
	dbOnce.Do(func() {
		// log.Info().Msg("Starting postgres")

		dbHost := os.Getenv("DB_HOST")
		dbPort := os.Getenv("DB_PORT")
		dbUser := os.Getenv("DB_USER")
		dbPass := os.Getenv("DB_PASSWORD")
		dbName := os.Getenv("DB_NAME")

		dbConnStr = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=America/Sao_Paulo", dbHost, dbUser, dbPass, dbName, dbPort)
		var err error
		dbInstance, err = gorm.Open(postgres.Open(dbConnStr), &gorm.Config{})
		if err != nil {
			log.Fatal().Err(err).Msg("Could not open/create " + dbConnStr)
			return
		}

		fmt.Println("Connected to database")
	})

	if dbInstance == nil {
		return nil, "", fmt.Errorf("could not establish database connection")
	}

	return dbInstance, dbConnStr, nil
}

func startSqlite(exPath string) (*gorm.DB, string, error) {
	// log.Info().Msg("Starting sqlite")

	db, err := gorm.Open(sqlite.Open(exPath+"/dbdata/users.db"), &gorm.Config{})
	if err != nil {
		log.Fatal().Err(err).Msg("Could not open/create " + exPath + "/dbdata/users.db")
		return nil, "", err
	}

	db.AutoMigrate(&User{}, &UserHistory{})

	return db, exPath + "/dbdata/users.db", nil
}

func NewService(exPath string, driver string) (Service, string, error) {
	var err error
	var db *gorm.DB
	var connString string

	switch driver {
	case "mysql":
		db, connString, err = startMysql()
	case "postgres":
		db, connString, err = startPostgres()
	default:
		db, connString, err = startSqlite(exPath)
	}

	db.AutoMigrate(&User{}, &UserHistory{})

	if err != nil {
		return nil, "", err
	}

	s := &service{db: db}

	return s, connString, nil
}

func (s *service) CreateUser(user *User) (int, error) {

	result := s.db.Create(user)

	if result.Error != nil {
		log.Error().Err(result.Error).Msg("Could not create user")

		return 0, result.Error
	}

	return int(user.ID), nil
}

func (s *service) UpdateUser(user *User) error {

	result := s.db.Save(user)

	if result.Error != nil {
		log.Error().Err(result.Error).Msg("Could not update user")

		return result.Error
	}

	return nil
}
func (s *service) SetQrcode(id int, qrcode string, instance string) error {
	// log.Info().Msgf("Attempting to set QR code for user %d with instance %s", id, instance)
	result := s.db.Model(&User{}).Where("id = ?", id).Where("instance = ?", instance).Update("qrcode", qrcode)
	if result.Error != nil {
		log.Error().Err(result.Error).Msgf("Could not set qrcode for user %d with instance %s", id, instance)
		return result.Error
	}

	if result.RowsAffected == 0 {
		log.Warn().Msgf("No rows affected when setting QR code for user %d with instance %s", id, instance)
		return fmt.Errorf("no rows affected")
	}

	// log.Info().Msgf("Successfully set QR code for user %d with instance %s", id, instance)
	return nil
}

func (s *service) SetWebhook(id int, webhook string) error {

	err := s.db.Model(&User{}).Where("id = ?", id).Update("webhook", webhook).Error

	if err != nil {
		log.Error().Err(err).Msg("Could not set webhook")

		return err
	}

	return nil
}

func (s *service) SetConnected(id int) error {

	err := s.db.Model(&User{}).Where("id = ?", id).Update("connected", 1).Error

	if err != nil {
		log.Error().Err(err).Msg("Could not set user as connected")

		return err
	}

	return nil
}

func (s *service) SetDisconnected(id int) error {

	err := s.db.Model(&User{}).Where("id = ?", id).Update("connected", 0).Error

	if err != nil {
		log.Error().Err(err).Msg("Could not set user as disconnected")

		return err
	}

	return nil
}

func (s *service) SetJid(id int, jid string) error {

	err := s.db.Model(&User{}).Where("id = ?", id).Update("jid", jid).Error

	if err != nil {
		log.Error().Err(err).Msg("Could not set jid")

		return err
	}

	return nil
}

func (s *service) SetEvents(id int, events string) error {

	err := s.db.Model(&User{}).Where("id = ?", id).Update("events", events).Error

	if err != nil {
		log.Error().Err(err).Msg("Could not set events")

		return err
	}

	return nil
}

func (s *service) SetPairingCode(id int, pairingCode string, instance string) error {

	err := s.db.Model(&User{}).Where("id = ?", id).Where("instance = ?", instance).Update("pairing_code", pairingCode).Error

	if err != nil {
		log.Error().Err(err).Msg("Could not set pairing code")

		return err
	}

	return nil
}

// SetCountMsg incrementa o contador de mensagens diárias do usuário
func (s *service) SetCountMsg(userID uint, typeMsg string) error {
	// Definir a data atual
	today := time.Now().Truncate(24 * time.Hour)

	// Encontrar ou criar o registro para o dia atual
	var userHistory UserHistory
	err := s.db.Where("user_id = ? AND date = ?", userID, today).First(&userHistory).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Criar novo registro se não encontrado
			userHistory = UserHistory{
				UserID: userID,
				Date:   today,
			}
			s.db.Create(&userHistory)
		} else {
			fmt.Println("Erro ao buscar UserHistory:", err)
			return err
		}
	}

	if typeMsg == "online" {
		userHistory.IsOnline = true
		err = s.db.Model(&userHistory).Update("is_online", true).Error
		if err != nil {
			fmt.Println("Erro ao atualizar status online:", err)
			return err
		}
	} else {
		// Incrementar o campo correto
		column := fmt.Sprintf("count_%s_msg", typeMsg)
		err = s.db.Model(&userHistory).Update(column, gorm.Expr(fmt.Sprintf("%s + ?", column), 1)).Error
		if err != nil {
			fmt.Println("Erro ao incrementar contagem de mensagens:", err)
			return err
		}
	}
	return nil
}

func (s *service) GetUserById(id int) (*User, error) {
	var user User

	err := s.db.Where("id = ?", id).First(&user).Error

	if err != nil {
		log.Error().Err(err).Msg("Could not get user")
		return nil, err
	}

	return &user, nil
}

func (s *service) GetUserByToken(token string) (*User, error) {
	var user User

	err := s.db.Where("token = ?", token).First(&user).Error

	if err != nil {
		log.Error().Err(err).Msg("Could not get user")
		return nil, err
	}

	return &user, nil
}

func (s *service) ListConnectedUsers() ([]*User, error) {
	var users []*User
	instance := os.Getenv("INSTANCE")

	if instance == "" {
		panic("INSTANCE is not set")
	}

	err := s.db.Where("connected = ? AND instance = ?", 1, instance).Find(&users).Error

	if err != nil {
		log.Error().Err(err).Msg("Could not list users")

		return nil, err
	}

	return users, nil
}

func (s *service) DeleteUser(id int) error {

	err := s.db.Delete(&User{}, id).Error

	if err != nil {
		log.Error().Err(err).Msg("Could not delete user")

		return err
	}

	return nil
}
