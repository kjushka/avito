package main

import (
	"avito_test/controller"
	"database/sql"
	"fmt"
	"github.com/go-martini/martini"
	_ "github.com/lib/pq"
	"net/http/pprof"
)

const DSN = "user=root password=root dbname=root sslmode=disable"

func newServer(db *sql.DB) *martini.ClassicMartini {
	c := controller.NewController(db)
	go c.ListenControllerChans()
	m := martini.Classic()
	m.Get("/proc", c.GetProcStatus)
	m.Get("/offers", c.FindOffersByParams)
	m.Post("/send", c.ReadFileFromRequest)
	m.Any("/debug/pprof/", pprof.Index)
	m.Any("/debug/pprof/cmdline", pprof.Cmdline)
	m.Any("/debug/pprof/profile", pprof.Profile)
	m.Any("/debug/pprof/symbol", pprof.Symbol)
	m.Any("/debug/pprof/trace", pprof.Trace)
	return m
}

func main() {
	/*name := os.Getenv("DATABASE_NAME")
	user := os.Getenv("DATABASE_USER")
	pass := os.Getenv("DATABASE_PASS")
	host := os.Getenv("DATABASE_HOST")
	log.Println(name, user, pass)
	dsn := fmt.Sprintf(
		"host=%v port=5432 user=%v dbname=%v password=%v sslmode=disable",
		host,
		user,
		name,
		pass,
	)*/

	db, err := sql.Open("postgres", DSN)
	if err != nil {
		panic(err)
	}
	db.Ping()
	defer db.Close()
	fmt.Println("Connected to db")

	m := newServer(db)
	m.RunOnAddr(":8080")
}
