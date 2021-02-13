package controller

import (
	"avito_test/model"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/tealeg/xlsx"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

type Controller struct {
	db                   *sql.DB
	procNumber           int64
	goroutine2Status     map[int64]string
	xlsxRequestWorkerMap map[int64]*xlsxRequestWorker
	upsertedChan         chan *countData `json: "-"`
	//updatedChan          chan *countData `json: "-"`
	deletedChan      chan *countData `json: "-"`
	errorStringsChan chan *errorData `json: "-"`
	mutex            *sync.Mutex
	waitChan         chan struct{}
}

type countData struct {
	goroutineNum int64
	count        int64
}
type errorData struct {
	goroutineNum int64
	errorStr     string
}

type xlsxRequestWorker struct {
	//количество созданных товаров, обновлённых, удалённых и количество строк с ошибками
	SenderId     int64
	Created      int64
	Updated      int64
	Deleted      int64
	ErrorStrings []string
}

func (c *Controller) inc() {
	c.procNumber = c.procNumber + 1
}

func NewController(db *sql.DB) *Controller {
	return &Controller{
		db:                   db,
		procNumber:           0,
		goroutine2Status:     make(map[int64]string),
		xlsxRequestWorkerMap: make(map[int64]*xlsxRequestWorker),
		upsertedChan:         make(chan *countData),
		//updatedChan:          make(chan *countData),
		deletedChan:      make(chan *countData),
		errorStringsChan: make(chan *errorData),
		mutex:            &sync.Mutex{},
		waitChan:         make(chan struct{}),
	}
}

func (c *Controller) GetProcStatus(r *http.Request) (int, string) {
	defer c.mutex.Unlock()
	number, err := strconv.ParseInt(r.FormValue("number"), 10, 64)
	if err != nil {
		log.Println("error in atoi:", err)
		return 500, err.Error()
	}
	c.mutex.Lock()
	if status, ok := c.goroutine2Status[number]; ok {
		return 200, status
	} else {
		return 500, "incorrect procedure number"
	}
}

func (c *Controller) FindProductByParams(w http.ResponseWriter, r *http.Request) (int, string) {
	sellerId := r.FormValue("seller")
	offerId := r.FormValue("offer")
	name := r.FormValue("name")
	sqlQueryParams := []string{}
	if sellerId != "" {
		if _, err := strconv.ParseInt(sellerId, 10, 64); err != nil {
			log.Println("error in parsing seller id:", err.Error())
			return 500, err.Error()
		} else {
			sqlQueryParams = append(sqlQueryParams, fmt.Sprintf("seller_id = %v", sellerId))
		}
	}
	if offerId != "" {
		if _, err := strconv.ParseInt(offerId, 10, 64); err != nil {
			log.Println("error in parsing offer id:", err.Error())
			return 500, err.Error()
		} else {
			sqlQueryParams = append(sqlQueryParams, fmt.Sprintf("offer_id = %v", offerId))
		}
	}
	if name != "" {
		sqlQueryParams = append(sqlQueryParams, fmt.Sprintf("name ilike '%v%v%v'", "%", name, "%"))
	}

	query := fmt.Sprintf("select seller_id, offer_id, name, price, quantity from product where %v",
		strings.Join(sqlQueryParams, " and "))

	rows, err := c.db.Query(
		query,
	)
	if err != nil && err != sql.ErrNoRows {
		log.Println("error in select query:", err)
		return 500, err.Error()
	}

	products := []*model.Product{}
	for rows.Next() {
		pr := &model.Product{}
		err = rows.Scan(
			&pr.SellerId,
			&pr.OfferId,
			&pr.Name,
			&pr.Price,
			&pr.Quantity,
		)
		if err != nil {
			log.Println("error in scanning rows:", err.Error())
			return 500, err.Error()
		}
		products = append(products, pr)
	}

	w.Header().Set("Content-Type", "application/json")
	return c.makeContentResponse(200, products)
}

func (c *Controller) ReadFileFromRequest(r *http.Request) (int, string) {
	senderId, err := strconv.ParseInt(r.FormValue("seller"), 10, 64)
	if err != nil {
		log.Println("error in parsing sender id:", err.Error())
		return 500, err.Error()
	}

	c.goroutine2Status[c.procNumber] = "new"
	c.xlsxRequestWorkerMap[c.procNumber] = &xlsxRequestWorker{
		SenderId: senderId,
	}

	log.Println("File Upload Endpoint Hit")

	//Max size of file set to 120MB
	r.ParseMultipartForm(120 << 20)
	file, handler, err := r.FormFile("file")
	if err != nil {
		log.Println("error retrieving the file:", err)
		c.mutex.Lock()
		c.goroutine2Status[c.procNumber] = fmt.Sprintf("error: %v", err.Error())
		c.mutex.Unlock()
		return 500, err.Error()
	}

	go c.workWithTempFile(file, handler, c.procNumber)
	defer c.inc()
	return 200, strconv.FormatInt(c.procNumber, 10)
}

func (c *Controller) ListenControllerChans() {
	for {
		select {
		case cData := <-c.upsertedChan:
			c.mutex.Lock()
			worker := c.xlsxRequestWorkerMap[cData.goroutineNum]
			worker.Created += cData.count
			c.mutex.Unlock()
			c.waitChan <- struct{}{}
		case cData := <-c.deletedChan:
			c.mutex.Lock()
			worker := c.xlsxRequestWorkerMap[cData.goroutineNum]
			worker.Deleted += cData.count
			c.mutex.Unlock()
			c.waitChan <- struct{}{}
		case cData := <-c.errorStringsChan:
			c.mutex.Lock()
			worker := c.xlsxRequestWorkerMap[cData.goroutineNum]
			worker.ErrorStrings = append(worker.ErrorStrings, cData.errorStr)
			c.mutex.Unlock()
			c.waitChan <- struct{}{}
		}
	}
}

func (c *Controller) workWithTempFile(file multipart.File, handler *multipart.FileHeader, grnumber int64) {
	defer file.Close()
	log.Printf("Uploaded File: %+v\n", handler.Filename)
	log.Printf("File Size: %+v\n", handler.Size)
	wg := &sync.WaitGroup{}
	fileParams := strings.Split(handler.Filename, ".")
	if fileParams[len(fileParams)-1] != "xlsx" {
		err := fmt.Errorf("unsupported file type: %v", fileParams[len(fileParams)-1])
		log.Println(err.Error())
		c.mutex.Lock()
		c.goroutine2Status[grnumber] = fmt.Sprintf("error: %v", err.Error())
		c.mutex.Unlock()
		return
	}

	tempFile, err := ioutil.TempFile("temp_files", "upload-*.xlsx")
	if err != nil {
		err := fmt.Errorf("error in creating temp file: %v", err)
		log.Println(err.Error())
		c.mutex.Lock()
		c.goroutine2Status[grnumber] = fmt.Sprintf("error: %v", err.Error())
		c.mutex.Unlock()
		return
	}
	defer tempFile.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		err := fmt.Errorf("error in reading file: %v", err.Error())
		log.Println(err.Error())
		c.mutex.Lock()
		c.goroutine2Status[grnumber] = fmt.Sprintf("error: %v", err.Error())
		c.mutex.Unlock()
		return
	}

	tempFile.Write(bytes)
	c.goroutine2Status[grnumber] = "file prepared for using"

	wg.Add(1)
	go c.readAndParseXLSXFile(wg, tempFile.Name(), grnumber)

	wg.Wait()

	err = os.Remove(tempFile.Name())

	c.mutex.Lock()
	worker := c.xlsxRequestWorkerMap[grnumber]
	finishStr := fmt.Sprintf(
		"finished with result: created or updated - %v,\ndeleted - %v,\nerrors - %v",
		worker.Created,
		worker.Deleted,
		strings.Join(worker.ErrorStrings, ",\n"),
	)

	c.goroutine2Status[grnumber] = finishStr
	c.mutex.Unlock()

	if err != nil {
		log.Println("error in deleting file:", err)
		return
	}

	return
}

func (c *Controller) readAndParseXLSXFile(fileWg *sync.WaitGroup, filename string, grnumber int64) {
	defer fileWg.Done()

	xlsxFile, err := xlsx.OpenFile(fmt.Sprintf("%v", filename))
	if err != nil {
		err := fmt.Errorf("error in opening xlsx file: %v", err)
		log.Println(err)
		c.mutex.Lock()
		c.goroutine2Status[grnumber] = fmt.Sprintf("error: %v", err.Error())
		c.mutex.Unlock()
		return
	}

	sheetWg := &sync.WaitGroup{}
	//ctx, cancelFunc := context.WithCancel(context.Background())
	for _, sheet := range xlsxFile.Sheets {
		sheetWg.Add(1)
		go c.parseSheet(sheetWg, sheet, grnumber)
	}

	c.mutex.Lock()
	c.goroutine2Status[grnumber] = "working with sheets"
	c.mutex.Unlock()

	sheetWg.Wait()
}

func (c *Controller) parseSheet(sheetWg *sync.WaitGroup, sheet *xlsx.Sheet, grnumber int64) {
	defer sheetWg.Done()

	rows := make([]*xlsx.Row, 100)
	rowsWg := &sync.WaitGroup{}
	lastNumber := 0
	for i, row := range sheet.Rows {
		rows[i%100] = row
		lastNumber = i % 100
		if (i+1)%100 == 0 {
			rowsWg.Add(1)
			goRows := make([]*xlsx.Row, 100)
			copy(goRows, rows)
			go c.workWithRows(rowsWg, goRows, lastNumber, grnumber)

		}
	}
	rowsWg.Add(1)
	go c.workWithRows(rowsWg, rows, lastNumber, grnumber)

	rowsWg.Wait()
}

func (c *Controller) workWithRows(rowsWs *sync.WaitGroup, rows []*xlsx.Row, lastNumber int, grnumber int64) {
	defer rowsWs.Done()
	deleteData := []string{}
	upsertData := []string{}
	for i := 0; i <= lastNumber; i++ {
		offerId, err := strconv.Atoi(rows[i].Cells[0].Value)
		if err != nil {
			err := fmt.Sprintf("row %v: error in atoi offer id: %v", rows[i].Cells, err)
			log.Println(err)
			c.errorStringsChan <- &errorData{
				goroutineNum: grnumber,
				errorStr:     err,
			}
			continue
		}
		if offerId <= 0 {
			err := fmt.Sprintf("row %v: offer id lower or equals zero", rows[i].Cells)
			log.Println(err)
			c.errorStringsChan <- &errorData{
				goroutineNum: grnumber,
				errorStr:     err,
			}
			continue
		}

		available, err := strconv.ParseBool(strings.ToLower(rows[i].Cells[4].Value))
		if err != nil {
			err := fmt.Sprintf("row %v: error in parsing available: %v", rows[i].Cells, err)
			log.Println(err)
			c.errorStringsChan <- &errorData{
				goroutineNum: grnumber,
				errorStr:     err,
			}
			continue
		}
		if !available {
			deleteData = append(deleteData, strconv.Itoa(offerId))
			continue
		}

		name := rows[i].Cells[1].Value

		price, err := strconv.Atoi(rows[i].Cells[2].Value)
		if err != nil {
			err := fmt.Sprintf("row %v: error in atoi price: %v", rows[i].Cells, err)
			log.Println(err)
			c.errorStringsChan <- &errorData{
				goroutineNum: grnumber,
				errorStr:     err,
			}
			continue
		}
		if price < 0 {
			err := fmt.Sprintf("row %v: price lower than zero", rows[i].Cells)
			log.Println(err)
			c.errorStringsChan <- &errorData{
				goroutineNum: grnumber,
				errorStr:     err,
			}
			continue
		}

		quantity, err := strconv.Atoi(rows[i].Cells[3].Value)
		if err != nil {
			err := fmt.Sprintf("row %v: error in atoi quantity: %v", rows[i].Cells, err)
			log.Println(err)
			c.errorStringsChan <- &errorData{
				goroutineNum: grnumber,
				errorStr:     err,
			}
			continue
		}
		if quantity < 0 {
			err := fmt.Sprintf("row %v: quantity lower than zero", rows[i].Cells)
			log.Println(err)
			c.errorStringsChan <- &errorData{
				goroutineNum: grnumber,
				errorStr:     err,
			}
			continue
		}

		upsertData = append(upsertData, fmt.Sprintf(
			"(%v, %v, '%v', %v, %v, %v)",
			strconv.FormatInt(c.xlsxRequestWorkerMap[grnumber].SenderId, 10),
			strconv.Itoa(offerId),
			name,
			strconv.Itoa(price),
			strconv.Itoa(quantity),
			strconv.FormatBool(available),
		))
	}

	if len(upsertData) != 0 {
		result, err := c.db.Exec(
			fmt.Sprintf("insert into product (seller_id, offer_id, name, price, quantity, available) "+
				"values %v on conflict on constraint product_id do update set name = excluded.name, "+
				"price = excluded.price, quantity = excluded.quantity, available = excluded.available;",
				strings.Join(upsertData, ", ")))
		if err != nil {
			log.Println("error in upsert data:", err)
			return
		}
		rowsUpserted, _ := result.RowsAffected()
		c.upsertedChan <- &countData{
			goroutineNum: grnumber,
			count:        rowsUpserted,
		}
		<-c.waitChan
	}

	if len(deleteData) != 0 {
		result, err := c.db.Exec(fmt.Sprintf("delete from product where offer_id in (%v) and seller_id = %v",
			strings.Join(deleteData, ", "), c.xlsxRequestWorkerMap[grnumber].SenderId))
		if err != nil {
			log.Println("error in delete data:", err)
			return
		}
		rowsDeleted, _ := result.RowsAffected()
		c.deletedChan <- &countData{
			goroutineNum: grnumber,
			count:        rowsDeleted,
		}
		<-c.waitChan
	}
}

func (c *Controller) makeContentResponse(code int, content interface{}) (int, string) {
	byteResponse, err := json.Marshal(content)
	if err != nil {
		log.Println("Error during marshalling", err.Error())
		return 500, err.Error()
	}
	return code, string(byteResponse)
}
