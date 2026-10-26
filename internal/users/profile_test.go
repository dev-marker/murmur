package users_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fiston-user/murmur/internal/platform/replica"
	"github.com/fiston-user/murmur/internal/platform/testdb"
	"github.com/fiston-user/murmur/internal/timeline"
	"github.com/fiston-user/murmur/internal/users"
)

// Who a token is, a profile for its owner and for others, and who to follow.
func TestProfiles(t *testing.T) {
	db, rdb := testdb.New(t), testdb.Redis(t)
	u := users.New(db, timeline.New(db, rdb, 1000))
	p := users.NewProfiles(db, replica.New(db, nil, rdb))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /users", u.Create)
	mux.HandleFunc("POST /users/{handle}/follow", u.Auth(u.Follow))
	mux.HandleFunc("GET /me", u.Auth(p.Me))
	mux.HandleFunc("GET /users/{handle}", u.Maybe(p.Show))
	mux.HandleFunc("GET /suggestions", u.Auth(p.Suggest))
	// call sends one request and returns the status and the body, without its newline
	call := func(method, path, token, body string) (int, string) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code, strings.TrimSpace(rec.Body.String())
	}
	tokens := map[string]string{}
	for _, h := range []string{"alice", "bob", "carol", "dave", "erin"} {
		_, body := call("POST", "/users", "", fmt.Sprintf(`{"handle": %q}`, h))
		var out struct{ Token string }
		json.Unmarshal([]byte(body), &out)
		tokens[h] = out.Token
	}
	// alice is followed by three, bob by two, carol by one. alice follows bob.
	for _, f := range [][2]string{{"bob", "alice"}, {"carol", "alice"}, {"dave", "alice"},
		{"carol", "bob"}, {"alice", "bob"}, {"dave", "carol"}} {
		call("POST", "/users/"+f[1]+"/follow", tokens[f[0]], "")
	}
	db.Exec(context.Background(), `insert into posts (user_id, body) values (1, 'a'), (1, 'b')`)

	for _, tc := range []struct {
		name, path, token string
		status            int
		want              string // the body, without "joined"
	}{
		{"who am I", "/me", tokens["bob"], 200, `{"handle":"bob","id":2}`},
		{"a profile I follow", "/users/alice", tokens["bob"], 200,
			`{"followed_by_me":true,"followers":3,"following":1,"handle":"alice","posts":2}`},
		{"one I don't", "/users/carol", tokens["bob"], 200,
			`{"followed_by_me":false,"followers":1,"following":2,"handle":"carol","posts":0}`},
		{"for nobody", "/users/alice", "", 200,
			`{"followed_by_me":false,"followers":3,"following":1,"handle":"alice","posts":2}`},
		{"nobody by that name", "/users/zed", "", 404, `{"error":"no such user"}`},
		{"who to follow, for erin", "/suggestions", tokens["erin"], 200,
			`{"users":[{"followers":3,"handle":"alice"},{"followers":2,"handle":"bob"},` +
				`{"followers":1,"handle":"carol"}]}`},
		{"not the ones dave follows", "/suggestions", tokens["dave"], 200,
			`{"users":[{"followers":2,"handle":"bob"},{"followers":0,"handle":"erin"}]}`},
	} {
		status, body := call("GET", tc.path, tc.token, "")
		var got map[string]any
		json.Unmarshal([]byte(body), &got)
		delete(got, "joined")
		b, _ := json.Marshal(got)
		if status != tc.status || string(b) != tc.want {
			t.Errorf("%s: got %d %s, want %d %s", tc.name, status, b, tc.status, tc.want)
		}
	}
}
