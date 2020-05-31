package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type User struct {
	gorm.Model
	Nickname string `gorm:"type:varchar(20);unique;not null"`
}

type UserRepository interface {
	AddUser(User) error
	GetByNickname(string) (*User, error)
	ChangeNickname(User, string) error
}

type UserDB struct {
	db *gorm.DB
}

func (u UserDB) AddUser(user User) error {
	return u.db.Create(&user).Error
}

func (u UserDB) GetByNickname(nick string) (*User, error) {
	var user User
	if u.db.First(&user, User{Nickname: nick}).RecordNotFound() {
		return nil, fmt.Errorf("no user found")
	}
	return &user, nil
}

func (u UserDB) ChangeNickname(user User, newNick string) error {
	return u.db.Model(&user).Update("Nickname", newNick).Error
}

func ConnectToDB(dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database failed with error: %w", err)
	}
	if err = db.AutoMigrate(&User{}).Error; err != nil {
		return nil, fmt.Errorf("migrating models failed with error: %w", err)
	}
	return db, nil
}

// Authentication shall establish a user's identity for an HTTP request.
// Assume it supports only HTTP Bearer authentication, but it can be abstracted
// into an interface for which several authentication mechanisms can be implemented.
type Authentication struct {
	currentUser *User
	userRepo    UserRepository
}

func (authn Authentication) userFromRequest(r *http.Request) (*User, error) {
	nick, _, ok := r.BasicAuth()
	if !ok {
		return nil, fmt.Errorf("no user credentials provided")
	}
	var err error
	authn.currentUser, err = authn.userRepo.GetByNickname(nick)

	return authn.currentUser, err
}

// ServeHTTP allows us to use Authentication as an http.Handler and to compose it
// with other request handlers as a middleware. We don't do that here, but we write
// this method to demonstrate that it's possible to fall back to http.Handler composition.
//func (authn Authentication) middleware(next http.Handler) http.Handler {
//	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		var err error
//		if authn.currentUser, err = authn.userFromRequest(r); err != nil {
//			// end request if authentication credentials are invalid
//		}
//	})
//}

type UserAccountController struct {
	userRepo UserRepository
}

func NewUserAccountcontroller(db *gorm.DB) UserAccountController {
	return UserAccountController{userRepo: UserDB{db: db}}
}

func (rg UserAccountController) changeNickname(w http.ResponseWriter, r *http.Request) {
	user, err := Authentication{userRepo: rg.userRepo}.userFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	// Authentication is done at this point; authorisation can be performed here if necessary.
	// This approach makes it obvious in the request handler that authentication is needed.
	// Authorisation is often not as generic as authentication, so it makes sense to do it
	// in the request handler where we have more context about the operation (in this case
	// we know that a delete operation has been attempted.
	// If the authorisation rules can be generalised for operations on this resource, they can
	// be factored out into methods on the resource.
	pathSegments := strings.Split(r.URL.Path, "/")
	urlNick := strings.TrimSpace(pathSegments[len(pathSegments)-1])
	if urlNick != user.Nickname {
		http.Error(w, "You cannot change the Nickname for this account!", http.StatusUnauthorized)
		return
	}

	if err = r.ParseForm(); err != nil {
		http.Error(w, "Could not process your request", http.StatusBadRequest)
		return
	}
	newNick := r.PostFormValue("nickname")
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

	// Compare with the alternative
	//
	// First, the context value is set in a separate middleware. If by misconfiguration it happened
	// to be absent, we have to trace the problem to that handler configuration rather than look a
	// few lines up.
	// ctx := r.Context()
	// user := ctx.Value("user")
	// if user == nil {
	// 	// end request
	// }
	// Also, Context values are not type safe, so we cannot be certain that what we got is of the
	// correct type; do you trust programmer consistency that much? A type assertion is necessary.
	// This type assertion would be done in every handler where the user info is passed in Context.
	// _user, ok := user.(User)
	// if !ok {
	// 	// handle cast error
	// }
}

func main() {
	db, err := ConnectToDB("demo_db.sqlite3")
	if err != nil {
		panic(err)
	}

	// Any operation defined on UserAccountController will share the database connection.
	accounts := NewUserAccountcontroller(db)

	r := mux.NewRouter()

	// For middleware that do not pass values further down, we can use http.Handler composition.
	// We can even register Authentication on the Router as follows.
	// Of course this is not useful: we rarely want to check authentication without using the
	// user identification we get in the operation.
	//r.Use(Authentication{userRepo: &UserDB{db: db}}.middleware)
	// We pass the RestrictedGallery delete method here, because we use the value in the request context.
	r.HandleFunc("/user/{nick}", accounts.changeNickname).Methods(http.MethodPatch)

	server := http.Server{Handler: r, Addr: "localhost:8080", ReadTimeout: time.Second * 10}
	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}
