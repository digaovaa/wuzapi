package database

import (
	"fmt"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/rs/zerolog/log"
)

type Service interface {
	CreateUser(user *User) (int, error)
	UpdateUser(user *User) error
	DeleteUser(id int) error
	SetQrcode(id int, qrcode string) error
	SetWebhook(id int, webhook string) error
	SetConnected(id int) error
	SetDisconnected(id int) error
	SetJid(id int, jid string) error
	SetEvents(id int, events string) error
	GetUserById(id int) (*User, error)
	GetUserByToken(token string) (*User, error)
	ListConnectedUsers() ([]*User, error)
}

type User struct {
	gorm.Model
	ID         uint   `gorm:"primaryKey"`
	Name       string `gorm:"type:text;not null"`
	Token      string `gorm:"type:text;not null"`
	Webhook    string `gorm:"type:text;not null;default:''"`
	Jid        string `gorm:"type:text;not null;default:''"`
	Qrcode     string `gorm:"type:text;not null;default:''"`
	Connected  int    `gorm:"type:integer"`
	Expiration int    `gorm:"type:integer"`
	Events     string `gorm:"type:text;not null;default:'All'"`
}

type service struct {
	db *gorm.DB
}

func startMysql() (*gorm.DB, error) {
	log.Info().Msg("Starting mysql")

	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", dbUser, dbPass, dbHost, dbPort, dbName)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})

	if err != nil {
		log.Fatal().Err(err).Msg("Could not open/create " + dsn)
		return nil, err
	}

	return db, nil
}

func startPostgres() (*gorm.DB, error) {

	log.Info().Msg("Starting postgres")

	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	connString := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=America/Sao_Paulo", dbHost, dbUser, dbPass, dbName, dbPort)
	db, err := gorm.Open(postgres.Open(connString), &gorm.Config{})
	fmt.Println(connString)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not open/create " + connString)
		return nil, err
	}

	fmt.Println("Connected to database")
	return db, nil
}

func startSqlite(exPath string) (*gorm.DB, error) {
	log.Info().Msg("Starting sqlite")

	db, err := gorm.Open(sqlite.Open(exPath+"/dbdata/users.db"), &gorm.Config{})
	if err != nil {
		log.Fatal().Err(err).Msg("Could not open/create " + exPath + "/dbdata/users.db")
		return nil, err
	}

	db.AutoMigrate(&User{})

	return db, nil
}

func NewService(exPath string, driver string) (Service, error) {
	var err error
	var db *gorm.DB

	switch driver {
	case "mysql":
		db, err = startMysql()
	case "postgres":
		db, err = startPostgres()
	default:
		db, err = startSqlite(exPath)
	}

	db.AutoMigrate(&User{})

	if err != nil {
		return nil, err
	}

	s := &service{db: db}

	return s, nil
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

func (s *service) SetQrcode(id int, qrcode string) error {
	err := s.db.Model(&User{}).Where("id = ?", id).Update("qrcode", qrcode).Error

	if err != nil {
		log.Error().Err(err).Msg("Could not set qrcode")

		return err
	}

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

	err := s.db.Where("connected = ?", 1).Find(&users).Error

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
