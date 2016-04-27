package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GeertJohan/go.rice"
	"github.com/codegangsta/negroni"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx"
	"github.com/meatballhat/negroni-logrus"
	"github.com/sebest/xff"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	app         = kingpin.New("analytics", "Simple analytics server")
	addr        = app.Flag("addr", "Address to listen on.").Envar("ADDR").Default(":5050").String()
	databaseURL = app.Flag("database_url", "URL to connect to PostgreSQL").Envar("DATABASE_URL").String()
)

var schema = []string{
	`create table if not exists "events" ("userid" character varying, "time" timestamp, "remote" text, "url" text, "action" text, "vars" json)`,
}

type tracker struct {
	db *pgx.ConnPool
}

var insertEvent = `insert into "events" ("userid", "time", "remote", "url", "action", "vars") values ($1, $2, $3, $4, $5, $6)`

func (t *tracker) track(e event) error {
	d, err := json.Marshal(e.vars)
	if err != nil {
		return err
	}

	if _, err := t.db.Exec(insertEvent, e.userID, e.time, e.remote, e.url, e.action, string(d)); err != nil {
		return err
	}

	return nil
}

type event struct {
	userID string
	time   time.Time
	remote string
	url    string
	action string
	vars   map[string]interface{}
}

var u = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func main() {
	kingpin.MustParse(app.Parse(os.Args[1:]))

	box := rice.MustFindBox("public")

	c, err := pgx.ParseDSN(*databaseURL)
	if err != nil {
		panic(err)
	}

	db, err := pgx.NewConnPool(pgx.ConnPoolConfig{
		ConnConfig:     c,
		MaxConnections: 4,
	})
	if err != nil {
		panic(err)
	}

	for _, q := range schema {
		if _, err := db.Exec(q); err != nil {
			panic(err)
		}
	}

	t := tracker{db}

	m := mux.NewRouter()

	m.Path("/a/ev").Methods("POST").HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		id := context.Get(r, "userID").(string)

		pageURL := r.Header.Get("referer")
		if s := r.URL.Query().Get("url"); s != "" {
			pageURL = s
		}

		var v struct {
			Action string
			Vars   map[string]interface{}
		}

		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			panic(err)
		}

		t.track(event{
			userID: id,
			time:   time.Now(),
			remote: r.RemoteAddr,
			url:    pageURL,
			action: v.Action,
			vars:   addBasics(v.Vars, r),
		})
	})

	m.Path("/a/ws").HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		id := context.Get(r, "userID").(string)

		c, err := u.Upgrade(rw, r, nil)
		if err != nil {
			panic(err)
		}

		pageURL := r.Header.Get("referer")
		if s := r.URL.Query().Get("url"); s != "" {
			pageURL = s
		}

		t.track(event{
			userID: id,
			time:   time.Now(),
			remote: r.RemoteAddr,
			url:    pageURL,
			action: "ws-connect",
			vars:   addBasics(nil, r),
		})

		defer func() {
			t.track(event{
				userID: id,
				time:   time.Now(),
				remote: r.RemoteAddr,
				url:    pageURL,
				action: "ws-disconnect",
				vars:   addBasics(nil, r),
			})
		}()

		for {
			var v struct {
				Action string
				Vars   map[string]interface{}
			}

			if err := c.ReadJSON(&v); err != nil {
				break
			}

			t.track(event{
				userID: id,
				time:   time.Now(),
				remote: r.RemoteAddr,
				url:    pageURL,
				action: v.Action,
				vars:   addBasics(v.Vars, r),
			})
		}
	})

	m.Path("/a/a.js").Methods("GET").HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		id := context.Get(r, "userID").(string)

		rw.Header().Set("content-type", "application/javascript")
		rw.WriteHeader(http.StatusOK)

		if _, err := strings.NewReplacer("##USER_ID##", id).WriteString(rw, box.MustString("a.js")); err != nil {
			panic(err)
		}
	})

	n := negroni.New()

	n.UseFunc(xff.XFF)
	n.Use(negronilogrus.NewMiddleware())
	n.Use(negroni.NewRecovery())
	n.UseFunc(addUserID)
	n.UseHandler(m)

	fmt.Printf("# listening on %s\n", *addr)

	if err := http.ListenAndServe(*addr, n); err != nil {
		panic(err)
	}
}

func addBasics(m map[string]interface{}, r *http.Request) map[string]interface{} {
	if m == nil {
		m = make(map[string]interface{})
	}

	m["user_agent"] = r.Header.Get("user-agent")

	return m
}

func addUserID(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	var t string

	if t == "" {
		if c, _ := r.Cookie("t"); c != nil {
			if c.Value != "" {
				t = c.Value
			}
		}
	}

	if t == "" {
		if s := r.Header.Get("ETag"); s != "" {
			t = s
		}
	}

	if t == "" {
		b := make([]byte, 20)
		if _, err := rand.Read(b); err != nil {
			panic(err)
		}

		t = base64.StdEncoding.EncodeToString(b)
	}

	rw.Header().Add("Set-Cookie", "t="+t+"; Expires="+time.Now().Add(time.Hour*24*365).Format(time.RFC1123))
	rw.Header().Set("ETag", t)

	context.Set(r, "userID", t)

	next(rw, r)
}
