package main

import (
	"avito_test/controller"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
)

func initDbForTests() *sql.DB {
	dsn := "user=root password=root dbname=root sslmode=disable"

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	db.Ping()

	return db
}

func TestIncorrectProcNumber(t *testing.T) {
	c := controller.NewController(initDbForTests())
	defer c.DB.Close()

	req, err := http.NewRequest("GET", "/proc?number=0", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	getProcStatus := func(w http.ResponseWriter, r *http.Request) {
		code, response := c.GetProcStatus(r)
		w.WriteHeader(code)
		w.Write([]byte(response))
	}
	handler := http.HandlerFunc(getProcStatus)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusInternalServerError)
	}

	expected := `incorrect procedure number`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

func TestCorrectProcNumber(t *testing.T) {
	c := controller.NewController(initDbForTests())
	defer c.DB.Close()
	c.Goroutine2Status[0] = "new"

	req, err := http.NewRequest("GET", "/proc?number=0", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	getProcStatus := func(w http.ResponseWriter, r *http.Request) {
		code, response := c.GetProcStatus(r)
		w.WriteHeader(code)
		w.Write([]byte(response))
	}
	handler := http.HandlerFunc(getProcStatus)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := `new`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

func TestFindProduct(t *testing.T) {
	c := controller.NewController(initDbForTests())
	_, err := c.DB.Exec(
		"insert into product (seller_id, offer_id, name, price, quantity, available) values (0, 0, 'test', 1000, 1000, true);")
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("GET", "/offers?seller=0&offer=0&name=es", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	getOffers := func(w http.ResponseWriter, r *http.Request) {
		code, response := c.FindOffersByParams(w, r)
		w.WriteHeader(code)
		w.Write([]byte(response))
	}
	handler := http.HandlerFunc(getOffers)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := `[{"SellerId":0,"OfferId":0,"Name":"test","Price":1000,"Quantity":1000}]`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
	_, err = c.DB.Exec("delete from product where seller_id = 0 and offer_id = 0")
	if err != nil {
		t.Fatal(err)
	}
}

func TestFindNonExistentNameProduct(t *testing.T) {
	c := controller.NewController(initDbForTests())
	_, err := c.DB.Exec(
		"insert into product (seller_id, offer_id, name, price, quantity, available) values (0, 0, 'test', 1000, 1000, true);")
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("GET", "/offers?seller=0&offer=0&name=no", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	getOffers := func(w http.ResponseWriter, r *http.Request) {
		code, response := c.FindOffersByParams(w, r)
		w.WriteHeader(code)
		w.Write([]byte(response))
	}
	handler := http.HandlerFunc(getOffers)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := `[]`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
	_, err = c.DB.Exec("delete from product where seller_id = 0 and offer_id = 0")
	if err != nil {
		t.Fatal(err)
	}
}

func TestFindProductsBySeller(t *testing.T) {
	c := controller.NewController(initDbForTests())
	_, err := c.DB.Exec(
		"insert into product (seller_id, offer_id, name, price, quantity, available) values (0, 0, 'test', 1000, 1000, true), (0, 1, 'test', 1000, 1000, true), (0, 2, 'test', 1000, 1000, true);")
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("GET", "/offers?seller=0", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	getOffers := func(w http.ResponseWriter, r *http.Request) {
		code, response := c.FindOffersByParams(w, r)
		w.WriteHeader(code)
		w.Write([]byte(response))
	}
	handler := http.HandlerFunc(getOffers)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := `[{"SellerId":0,"OfferId":0,"Name":"test","Price":1000,"Quantity":1000},{"SellerId":0,"OfferId":1,"Name":"test","Price":1000,"Quantity":1000},{"SellerId":0,"OfferId":2,"Name":"test","Price":1000,"Quantity":1000}]`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
	_, err = c.DB.Exec("delete from product where seller_id = 0 and offer_id in (0, 1, 2);")
	if err != nil {
		t.Fatal(err)
	}
}

func TestIncorrectSellerNumber(t *testing.T) {
	c := controller.NewController(initDbForTests())
	defer c.DB.Close()
	c.Goroutine2Status[0] = "new"

	req, err := http.NewRequest("Post", "/send?seller=test", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	sendFile := func(w http.ResponseWriter, r *http.Request) {
		code, response := c.ReadFileFromRequest(r)
		w.WriteHeader(code)
		w.Write([]byte(response))
	}
	handler := http.HandlerFunc(sendFile)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusInternalServerError)
	}

	expected := `strconv.ParseInt: parsing "test": invalid syntax`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}
