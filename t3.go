package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type PrintFunc func(format string, a ...interface{}) (n int, err error)

type MktsQuote struct {
	Symbol   string  `json:"symbol"`
	Exchange string  `json:"exchange"`
	Date     string  `json:"date"`
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   float64 `json:"volume"`
}

type Quote struct {
	Name     string  `json:"name"`
	Symbol   string  `json:"symbol"`
	Exchange string  `json:"exchange"`
	Date     string  `json:"date"`
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   float64 `json:"volume"`
}

type CachedQuote struct {
	lastMod   time.Time
	jsonQuote string
}

// Lookup by symbol. Ex. _cachedQuote["SBUX"]
var _cachedQuote map[string]*CachedQuote

func init() {
	_cachedQuote = map[string]*CachedQuote{}
}

func createTables(newfile string) {
	if fileExists(newfile) {
		s := fmt.Sprintf("File '%s' already exists. Can't initialize it.\n", newfile)
		fmt.Printf(s)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", newfile)
	if err != nil {
		fmt.Printf("Error opening '%s' (%s)\n", newfile, err)
		os.Exit(1)
	}

	ss := []string{
		"CREATE TABLE user (user_id INTEGER PRIMARY KEY NOT NULL, username TEXT UNIQUE, password TEXT, active INTEGER NOT NULL, email TEXT);",
		"INSERT INTO user (user_id, username, password, active, email) VALUES (1, 'admin', '', 1, '');",
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("DB error (%s)\n", err)
		os.Exit(1)
	}
	for _, s := range ss {
		_, err := txexec(tx, s)
		if err != nil {
			tx.Rollback()
			log.Printf("DB error (%s)\n", err)
			os.Exit(1)
		}
	}
	err = tx.Commit()
	if err != nil {
		log.Printf("DB error (%s)\n", err)
		os.Exit(1)
	}
}

func main() {
	os.Args = os.Args[1:]
	sw, parms := parseArgs(os.Args)

	// [-i new_file]  Create and initialize db file
	if sw["i"] != "" {
		dbfile := sw["i"]
		if fileExists(dbfile) {
			s := fmt.Sprintf("File '%s' already exists. Can't initialize it.\n", dbfile)
			fmt.Printf(s)
			os.Exit(1)
		}
		createTables(dbfile)
		os.Exit(0)
	}

	// Need to specify a db file as first parameter.
	if len(parms) == 0 {
		s := `Usage:

Start webservice using database file:
	t2 <sites.db>

Initialize new database file:
	t2 -i <sites.db>
`
		fmt.Printf(s)
		os.Exit(0)
	}

	// Exit if db file doesn't exist.
	dbfile := parms[0]
	if !fileExists(dbfile) {
		s := fmt.Sprintf(`Sites database file '%s' doesn't exist. Create one using:
	wb -i <notes.db>
`, dbfile)
		fmt.Printf(s)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		fmt.Printf("Error opening '%s' (%s)\n", dbfile, err)
		os.Exit(1)
	}

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "./static/coffee.ico") })
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir("./"))))
	http.HandleFunc("/api/lookup/", lookupHandler(db))

	port := "8000"
	fmt.Printf("Listening on %s...\n", port)
	err = http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	log.Fatal(err)
}

//*** DB functions ***
func sqlstmt(db *sql.DB, s string) *sql.Stmt {
	stmt, err := db.Prepare(s)
	if err != nil {
		log.Fatalf("db.Prepare() sql: '%s'\nerror: '%s'", s, err)
	}
	return stmt
}
func sqlexec(db *sql.DB, s string, pp ...interface{}) (sql.Result, error) {
	stmt := sqlstmt(db, s)
	defer stmt.Close()
	return stmt.Exec(pp...)
}
func txstmt(tx *sql.Tx, s string) *sql.Stmt {
	stmt, err := tx.Prepare(s)
	if err != nil {
		log.Fatalf("tx.Prepare() sql: '%s'\nerror: '%s'", s, err)
	}
	return stmt
}
func txexec(tx *sql.Tx, s string, pp ...interface{}) (sql.Result, error) {
	stmt := txstmt(tx, s)
	defer stmt.Close()
	return stmt.Exec(pp...)
}

//*** Helper functions ***

// Helper function to make fmt.Fprintf(w, ...) calls shorter.
// Ex.
// Replace:
//   fmt.Fprintf(w, "<p>Some text %s.</p>", str)
//   fmt.Fprintf(w, "<p>Some other text %s.</p>", str)
// with the shorter version:
//   P := makeFprintf(w)
//   P("<p>Some text %s.</p>", str)
//   P("<p>Some other text %s.</p>", str)
func makeFprintf(w io.Writer) func(format string, a ...interface{}) (n int, err error) {
	return func(format string, a ...interface{}) (n int, err error) {
		return fmt.Fprintf(w, format, a...)
	}
}
func listContains(ss []string, v string) bool {
	for _, s := range ss {
		if v == s {
			return true
		}
	}
	return false
}
func fileExists(file string) bool {
	_, err := os.Stat(file)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}
func makePrintFunc(w io.Writer) func(format string, a ...interface{}) (n int, err error) {
	// Return closure enclosing io.Writer.
	return func(format string, a ...interface{}) (n int, err error) {
		return fmt.Fprintf(w, format, a...)
	}
}
func atoi(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
func idtoi(sid string) int64 {
	return int64(atoi(sid))
}
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func parseArgs(args []string) (map[string]string, []string) {
	switches := map[string]string{}
	parms := []string{}

	standaloneSwitches := []string{}
	definitionSwitches := []string{"i"}
	fNoMoreSwitches := false
	curKey := ""

	for _, arg := range args {
		if fNoMoreSwitches {
			// any arg after "--" is a standalone parameter
			parms = append(parms, arg)
		} else if arg == "--" {
			// "--" means no more switches to come
			fNoMoreSwitches = true
		} else if strings.HasPrefix(arg, "--") {
			switches[arg[2:]] = "y"
			curKey = ""
		} else if strings.HasPrefix(arg, "-") {
			if listContains(definitionSwitches, arg[1:]) {
				// -a "val"
				curKey = arg[1:]
				continue
			}
			for _, ch := range arg[1:] {
				// -a, -b, -ab
				sch := string(ch)
				if listContains(standaloneSwitches, sch) {
					switches[sch] = "y"
				}
			}
		} else if curKey != "" {
			switches[curKey] = arg
			curKey = ""
		} else {
			// standalone parameter
			parms = append(parms, arg)
		}
	}

	return switches, parms
}

func lookupHandler(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		sym := strings.ToUpper(r.FormValue("sym"))
		if sym == "" {
			http.Error(w, "sym required", 401)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		P := makeFprintf(w)

		cq := _cachedQuote[sym]
		if cq != nil {
			elapsedMins := time.Since(cq.lastMod) / time.Minute
			if elapsedMins < 1 {
				// Return cached quote
				log.Printf("(Returning cached quote)\n")
				P(cq.jsonQuote)
				return
			}
		}

		log.Printf("*** Requesting new quote ***\n")

		mktsKey := "875d5614925e6d98037cbc8592b7bdc2"
		sreq := fmt.Sprintf("http://api.marketstack.com/v1/tickers/%s/eod/latest?access_key=%s", sym, mktsKey)
		req, err := http.NewRequest("GET", sreq, nil)
		if err != nil {
			panic(err)
		}
		c := http.Client{}
		res, err := c.Do(req)
		if err != nil {
			panic(err)
		}
		defer res.Body.Close()

		bs, err := ioutil.ReadAll(res.Body)
		if err != nil {
			panic(err)
		}

		var mq MktsQuote
		err = json.Unmarshal(bs, &mq)
		if err != nil {
			panic(err)
		}

		var q Quote
		q.Name = mq.Symbol
		q.Symbol = mq.Symbol
		q.Exchange = mq.Exchange
		q.Date = mq.Date
		q.Open = mq.Open
		q.High = mq.High
		q.Low = mq.Low
		q.Close = mq.Close
		q.Volume = mq.Volume

		bs, err = json.MarshalIndent(q, "", "\t")
		if err != nil {
			panic(err)
		}
		P(string(bs))

		_cachedQuote[q.Symbol] = &CachedQuote{lastMod: time.Now(), jsonQuote: string(bs)}
	}
}
