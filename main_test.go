package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
)

func sqliteInMemory(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := connectToDB(":memory:")
	if err != nil {
		t.Fatalf("expected to open database successfully, got error %v", err)
	}
	t.Cleanup(func() {
		if err = db.Close(); err != nil {
			t.Logf("closing database returned error: %v", err)
		}
	})

	userDB{db: db}.AddUser(userAccount{Nickname: "joe"})

	return db
}

func runChangeNicknameTest(path string, basicAuthUsername string, newNick string, expectedCode int, t *testing.T) {
	formData := url.Values{"nickname": {newNick}}.Encode()
	req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(formData))
	req.Header["Content-Type"] = []string{"application/x-www-form-urlencoded"}
	req.SetBasicAuth(basicAuthUsername, "")

	db := sqliteInMemory(t)
	accounts := newUserAccountcontroller(db)
	res := httptest.NewRecorder()

	accounts.changeNickname(res, req)

	if res.Code != expectedCode {
		t.Errorf("expected status code %d, got %d", expectedCode, res.Code)
	}
}

func Test_ChangeNickname_With_Explicit_Auth_Access(t *testing.T) {
	t.Run("Given incorrect credentials, it returns status 401", func(t *testing.T) {
		runChangeNicknameTest("/user/joe", "not_existing", "new_nick", http.StatusUnauthorized, t)
	})

	t.Run("Given correct credentials, it returns status 200", func(t *testing.T) {
		runChangeNicknameTest("/user/joe", "joe", "new_nick", http.StatusOK, t)
	})

	t.Run("Forbids a user from changing other user's nicknames", func(t *testing.T) {
		runChangeNicknameTest("/user/not_joe", "joe", "new_nick", http.StatusUnauthorized, t)
	})

	t.Run("Does not allow empty strings for nicknames", func(t *testing.T) {
		runChangeNicknameTest("/user/joe", "joe", "", http.StatusBadRequest, t)
	})
}

func runChangeNicknameCtxTest(path string, basicAuthUsername string, newNick string, expectedCode int, t *testing.T) {
	db := sqliteInMemory(t)
	r := setupRouter(db)

	// An interesting effect of the context approach is that
	// we cannot test its full execution path by using a ResponseRecorder.
	// We must create a server and send requests to it
	// in order to test the entire middleware configuration.
	server := httptest.NewServer(r)
	defer server.Close()

	formData := url.Values{"nickname": {newNick}}.Encode()
	req, err := http.NewRequest(http.MethodPut, server.URL+path, strings.NewReader(formData))
	if err != nil {
		t.Fatalf("constructing request failed with error: %v", err)
	}
	req.Header["Content-Type"] = []string{"application/x-www-form-urlencoded"}
	req.SetBasicAuth(basicAuthUsername, "")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("could not send request; error: %v", err)
	}
	_, err = ioutil.ReadAll(res.Body)
	err = res.Body.Close()
	if err != nil {
		t.Fatalf("could not read or close response body; error: %v", err)
	}

	if res.StatusCode != expectedCode {
		t.Errorf("expected status code %d, got %d", expectedCode, res.StatusCode)
	}
}

func Test_ChangeNickname_With_Implicit_Auth_Access_Via_Context(t *testing.T) {
	t.Run("Given incorrect credentials, it returns status 401", func(t *testing.T) {
		runChangeNicknameCtxTest("/user/joe", "not_existing", "new_nick", http.StatusUnauthorized, t)
	})

	t.Run("Given correct credentials, it returns status 200", func(t *testing.T) {
		runChangeNicknameCtxTest("/user/joe", "joe", "new_nick", http.StatusOK, t)
	})

	t.Run("Forbids a user from changing other user's nicknames", func(t *testing.T) {
		runChangeNicknameCtxTest("/user/not_joe", "joe", "new_nick", http.StatusUnauthorized, t)
	})

	t.Run("Does not allow empty strings for nicknames", func(t *testing.T) {
		runChangeNicknameCtxTest("/user/joe", "joe", "", http.StatusBadRequest, t)
	})
}
