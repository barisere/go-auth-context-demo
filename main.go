package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type userAccount struct {
	gorm.Model
	Nickname string `gorm:"type:varchar(20);unique;not null"`
}

type userRepository interface {
	AddUser(userAccount) error
	GetByNickname(string) (*userAccount, error)
	ChangeNickname(userAccount, string) error
}

type userDB struct {
	db *gorm.DB
}

func (u userDB) AddUser(user userAccount) error {
	return u.db.Create(&user).Error
}

func (u userDB) GetByNickname(nick string) (*userAccount, error) {
	var user userAccount
	if u.db.First(&user, userAccount{Nickname: nick}).RecordNotFound() {
		return nil, fmt.Errorf("no user found")
	}
	return &user, nil
}

func (u userDB) ChangeNickname(user userAccount, newNick string) error {
	return u.db.Model(&user).Update("Nickname", newNick).Error
}

func connectToDB(dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database failed with error: %w", err)
	}
	if err = db.AutoMigrate(&userAccount{}).Error; err != nil {
		return nil, fmt.Errorf("migrating models failed with error: %w", err)
	}
	return db, nil
}

// Authentication shall establish a user's identity for an HTTP request.
// It supports only HTTP Bearer authentication, but it can be extended to
// support several authentication mechanisms can be implemented.
type Authentication struct {
	userRepo userRepository
}

func (authn Authentication) userFromRequest(r *http.Request) (*userAccount, error) {
	nick, _, ok := r.BasicAuth()
	if !ok {
		return nil, fmt.Errorf("no user credentials provided")
	}
	return authn.userRepo.GetByNickname(nick)
}

type userCtxKey string

func (authn Authentication) setUserMidddleware(next http.Handler) http.Handler {
	handler := func(w http.ResponseWriter, r *http.Request) {
		user, err := authn.userFromRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey("user"), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(handler)
}

type userAccountController struct {
	userRepo userRepository
}

func newUserAccountcontroller(db *gorm.DB) userAccountController {
	return userAccountController{userRepo: userDB{db: db}}
}

func (rg userAccountController) changeNickname(w http.ResponseWriter, r *http.Request) {
	user, err := Authentication{userRepo: rg.userRepo}.userFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	pathSegments := strings.Split(r.URL.Path, "/")
	urlNick := strings.TrimSpace(pathSegments[len(pathSegments)-1])

	// Authentication is done at this point; authorisation can be performed here if necessary.
	// This approach makes it obvious in the request handler that authentication is needed.
	// Authorisation is often not as generic as authentication, so it makes sense to do it
	// in the request handler where we have more context about the operation (in this case
	// we know that a delete operation has been attempted.
	// If the authorisation rules can be generalised for operations on this resource, they can
	// be factored out into methods on the resource.
	if urlNick != user.Nickname {
		http.Error(w, "You cannot change the Nickname for this account!", http.StatusUnauthorized)
		return
	}

	newNick := r.FormValue("nickname")
	if newNick == "" {
		http.Error(w, "Nickname cannot be empty", http.StatusBadRequest)
		return
	}

	if err = rg.userRepo.ChangeNickname(*user, newNick); err != nil {
		log.Printf("error changing user Nickname: %v", err)
		http.Error(w, "Oops! Try again later.", http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Nickname successfully changed"))
	return
}

func (rg userAccountController) changeNicknameCtx(w http.ResponseWriter, r *http.Request) {
	// This approach uses the request context access the request user.
	//
	// First, the context value is set in a separate middleware. If by misconfiguration it happened
	// to be absent, we have to trace the problem to that handler configuration rather than look a
	// few lines up.
	// Also, Context values are not type safe, so we cannot be certain that what we got is of the
	// correct type; do you trust programmer consistency that much? A type assertion is necessary.
	// This type assertion would be done in every handler where the user info is passed in Context.
	user, ok := r.Context().Value(userCtxKey("user")).(*userAccount)
	if !ok {
		log.Println("'user' was not set in context")
		http.Error(w, "Oops. Please try again later.", http.StatusInternalServerError)
		return
	}

	pathSegments := strings.Split(r.URL.Path, "/")
	urlNick := strings.TrimSpace(pathSegments[len(pathSegments)-1])

	if urlNick != user.Nickname {
		http.Error(w, "You cannot change the Nickname for this account!", http.StatusUnauthorized)
		return
	}

	newNick := r.FormValue("nickname")
	if newNick == "" {
		http.Error(w, "Nickname cannot be empty", http.StatusBadRequest)
		return
	}

	if err := rg.userRepo.ChangeNickname(*user, newNick); err != nil {
		log.Printf("error changing user Nickname: %v", err)
		http.Error(w, "Oops! Try again later.", http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Nickname successfully changed"))
	return
}

func setupRouter(db *gorm.DB) *mux.Router {
	accounts := newUserAccountcontroller(db)

	r := mux.NewRouter()

	authn := Authentication{userRepo: userDB{db: db}}

	// For PUT requests we pass the request user in the request context.
	r.Handle(
		"/user/{nick}",
		authn.setUserMidddleware(http.HandlerFunc(accounts.changeNicknameCtx)),
	).Methods(http.MethodPut)

	// For PATCH requests we fetch the request user in the handler.
	r.HandleFunc("/user/{nick}", accounts.changeNickname).Methods(http.MethodPatch)

	return r
}

func main() {
	db, err := connectToDB("demo_db.sqlite3")
	if err != nil {
		panic(err)
	}

	r := setupRouter(db)

	server := http.Server{Handler: r, Addr: "localhost:8080", ReadTimeout: time.Second * 10}
	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}
