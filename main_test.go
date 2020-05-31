package main

import (
	"github.com/jinzhu/gorm"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func sqliteInMemory(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := ConnectToDB(":memory:")
	if err != nil {
		t.Fatalf("expected to open database successfully, got error %v", err)
	}
	t.Cleanup(func () {
		if err = db.Close(); err != nil {
			t.Logf("closing database returned error: %v", err)
		}
	})

	UserDB{db:db}.AddUser(User{Nickname: "joe"})

	return db
}

func runChangeNicknameTest(path string, basicAuthUsername string, newNick string, expectedCode int, t *testing.T) {
	req := httptest.NewRequest(http.MethodPatch, path, nil)
	req.PostForm = url.Values{ "nickname": []string{newNick}}
	req.SetBasicAuth(basicAuthUsername, "")
	rec := httptest.NewRecorder()
	db := sqliteInMemory(t)
	accounts := NewUserAccountcontroller(db)

	accounts.changeNickname(rec, req)

	if rec.Code != expectedCode {
		t.Errorf("expected status code %d, got %d", expectedCode, rec.Code)
	}
}

func Test_ChangeNickname_With_Incorrect_Auth_Fails(t *testing.T) {
	runChangeNicknameTest("/user/joe", "not_existing", "new_nick", http.StatusUnauthorized, t)
}

func Test_ChangeNickname_With_Correct_Auth_Succeeds(t *testing.T) {
	runChangeNicknameTest("/user/joe", "joe", "new_nick", http.StatusOK, t)
}

func Test_User_Cannot_Change_Other_User_Nickname(t *testing.T) {
	runChangeNicknameTest("/user/not_joe", "joe", "new_nick", http.StatusUnauthorized, t)
}

func Test_Nickname_Cannot_Be_Empty(t *testing.T) {
	runChangeNicknameTest("/user/joe", "joe", "", http.StatusBadRequest, t)
}

