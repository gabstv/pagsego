package pagsego

import (
	"bytes"
	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	ShippingPAC   = 1
	ShippingSEDEX = 2
	ShippingOther = 3
	XMLHeader     = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`
	CheckoutURL   = "https://ws.pagseguro.uol.com.br/v2/checkout"
)

type PaymentRequest struct {
	XMLName         xml.Name       `xml:"checkout"`
	Email           string         `xml:"email"`
	Token           string         `xml:"token"`
	Currency        string         `xml:"currency"`
	Items           []*PaymentItem `xml:"items>item"`
	ReferenceID     string         `xml:"reference"`
	Buyer           *Buyer         `xml:"sender"`
	Shipping        *Shipping      `xml:"shipping"`
	ExtraAmount     string         `xml:"extraAmount,omitempty"` // use this for discounts or taxes
	RedirectURL     string         `xml:"redirectURL,omitempty"`
	NotificationURL string         `xml:"notificationURL,omitempty"`
	MaxUses         string         `xml:"maxUses,omitempty"`  // from 0 to 999 (the amount of tries a user can do with the same reference ID)
	MaxAge          string         `xml:"maxAge,omitempty"`   // time (in seconds) that the returned payment code is valid (30-999999999)
	Metadata        []*Metadata    `xml:"metadata,omitempty"` // https://pagseguro.uol.com.br/v2/guia-de-integracao/api-de-pagamentos.html#v2-item-api-de-pagamentos-parametros-http
	IsSandbox       bool           `xml:"-"`                  // o PagSeguro não tem um modo sandbox no momento (╯°□°）╯︵ ┻━┻
}

type PaymentItem struct {
	XMLName      xml.Name `xml:"item"`
	Id           string   `xml:"id"`
	Description  string   `xml:"description"`
	PriceAmount  string   `xml:"amount"`
	Quantity     string   `xml:"quantity"`
	ShippingCost string   `xml:"shippingCost,omitempty"`
	Weight       string   `xml:"weight,omitempty"`
}

type Buyer struct {
	Email     string           `xml:"email"`
	Name      string           `xml:"name"`
	Phone     *Phone           `xml:"phone,omitempty"`
	Documents []*BuyerDocument `xml:"documents>document,omitempty"`
	BornDate  string           `xml:"bornDate,omitempty"` //dd/MM/yyyy optional
}

type Phone struct {
	AreaCode    string `xml:"areaCode,omitempty"` // optional
	PhoneNumber string `xml:"number,omitempty"`   // optional
}

type BuyerDocument struct {
	Type  string `xml:"type"` // It's always "CPF" ¯\_(ツ)_/¯
	Value string `xml:"value"`
}

type Shipping struct {
	Type    string           `xml:"type"`
	Cost    string           `xml:"cost"`
	Address *ShippingAddress `xml:"address,omitempty"`
}

type ShippingAddress struct {
	Country    string `xml:"country"`              // It's always "BRA" ¯\_(ツ)_/¯
	State      string `xml:"state,omitempty"`      // "SP"
	City       string `xml:"city,omitempty"`       // max 60 min 2
	PostalCode string `xml:"postalCode,omitempty"` // XXXXXXXX
	District   string `xml:"district,omitempty"`   // Bairro | max chars: 60
	Street     string `xml:"street,omitempty"`     // max: 80
	Number     string `xml:"number,omitempty"`     // max: 20
	Complement string `xml:"complement,omitempty"` // max: 40
}

type Metadata struct {
	Key   string     `xml:"key"`
	Value string     `xml:"value,omitempty"`
	Group []Metadata `xml:"group,omitempty"`
}

type ErrorResponse struct {
	Errors []XMLError `xml:"errors"`
}

type XMLError struct {
	XMLName xml.Name `xml:"error"`
	Code    int      `xml:"code"`
	Message string   `xml:"message"`
}

type PaymentPreResponse struct {
	XMLName xml.Name `xml:"checkout"`
	Code    string   `xml:"code"`
	Data    string   `xml:"data"`
}

type PaymentPreSubmitResult struct {
	CheckoutResponse *PaymentPreResponse
	Error            *ErrorResponse
	Success          bool
}

func NewPaymentRequest(sellerToken, sellerEmail, referenceID, redirectURL, notificationURL string) *PaymentRequest {
	req := &PaymentRequest{
		Email:           sellerEmail,
		Token:           sellerToken,
		Currency:        "BRL",
		ReferenceID:     referenceID,
		RedirectURL:     redirectURL,
		NotificationURL: notificationURL,
		MaxUses:         "10",
		MaxAge:          "7200",
	}
	return req
}

func (r *PaymentRequest) AddItem(id string, description string, amount float64, quantity int) *PaymentItem {
	item := &PaymentItem{
		Id:          id,
		Description: description,
		PriceAmount: toPriceAmountStr(amount),
		Quantity:    strconv.Itoa(quantity),
	}
	if r.Items == nil {
		r.Items = make([]*PaymentItem, 0)
	}
	r.Items = append(r.Items, item)

	return item
}

func (r *PaymentItem) SetWeight(grams int) *PaymentItem {
	r.Weight = strconv.Itoa(grams)
	return r
}

func (r *PaymentItem) SetAmount(amount float64) *PaymentItem {
	r.PriceAmount = toPriceAmountStr(amount)
	return r
}

func (r *PaymentItem) SetQuantity(quantity int) *PaymentItem {
	r.Quantity = strconv.Itoa(quantity)
	return r
}

func (r *PaymentItem) SetShippingCost(cost float64) *PaymentItem {
	r.ShippingCost = toPriceAmountStr(cost)
	return r
}

func (r *PaymentRequest) SetBuyer(name, email string) *Buyer {
	buyer := &Buyer{
		Name:  name,
		Email: email,
	}
	r.Buyer = buyer
	return buyer
}

func (r *Buyer) SetPhone(areaCode string, phone string) *Buyer {
	r.Phone = &Phone{
		AreaCode:    areaCode,
		PhoneNumber: phone,
	}
	return r
}

func (r *Buyer) SetCPF(cpf string) *Buyer {
	if r.Documents == nil {
		r.Documents = make([]*BuyerDocument, 0)
		r.Documents = append(r.Documents, &BuyerDocument{Type: "CPF"})
	}
	for i := 0; i < len(r.Documents); i++ {
		if r.Documents[i].Type == "CPF" {
			r.Documents[i].Value = cpf
			break
		}
	}
	return r
}

func (r *PaymentRequest) SetShipping(shippingType int, shippingCost float64) *Shipping {
	shipping := &Shipping{
		Type: strconv.Itoa(shippingType),
		Cost: toPriceAmountStr(shippingCost),
	}
	r.Shipping = shipping
	return shipping
}

func (r *Shipping) SetAddress(state, city, postalCode, district, street, number, complement string) *Shipping {
	addr := &ShippingAddress{
		Country:    "BRA",
		State:      state,
		City:       city,
		PostalCode: postalCode,
		District:   district,
		Street:     street,
		Number:     number,
		Complement: complement,
	}
	r.Address = addr
	return r
}

func (r *Shipping) SetAddressStateCity(state, city string) *Shipping {
	if r.Address == nil {
		r.SetAddress(state, city, "", "", "", "", "")
		return r
	}
	r.Address.State = state
	r.Address.City = city
	return r
}

func (r *Shipping) SetAddressCountry(country string) *Shipping {
	if r.Address == nil {
		r.SetAddress("", "", "", "", "", "", "")
	}
	r.Address.Country = country
	return r
}

func (r *PaymentRequest) Submit() (result *PaymentPreSubmitResult) {
	result = &PaymentPreSubmitResult{}

	// Conectar com timeout caso o PagSeguro esteja morgando
	functimeout := func(network, addr string) (net.Conn, error) {
		return net.DialTimeout(network, addr, time.Duration(30*time.Second))
	}

	// create a custom http client that ignores https cert validity, so we don't have to install PagSeguro CAs
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		Dial:            functimeout,
	}
	client := &http.Client{Transport: tr}

	// generate xml
	xmlb, err := xml.Marshal(r)

	if err != nil {
		log.Println("^~PAGSEGO~^ XML MARSHAL ERROR: " + err.Error())
		return
	}

	var clBuffer bytes.Buffer
	clBuffer.WriteString(XMLHeader)
	clBuffer.Write(xmlb)

	checkoutURL := fmt.Sprintf("%s?email=%s&token=%s&charset=%s", CheckoutURL, r.Email, r.Token, "UTF-8")

	// send the request (this goroutine is blocked until a response is received)
	resp, err := client.Post(checkoutURL, "application/xml", &clBuffer)

	if err != nil {
		log.Println("^~PAGSEGO~^ client.Post ERROR: " + err.Error())
		return
	}

	defer resp.Body.Close()
	clBuffer.Truncate(0)

	// io.Copy has a 32kB max buffer size, so no extra memory is consumed
	io.Copy(&clBuffer, resp.Body)
	respBytes := clBuffer.Bytes()
	log.Println(string(respBytes))
	var decoder *xml.Decoder

	errors := &ErrorResponse{}

	clBuffer.Truncate(0)
	clBuffer.Write(respBytes)
	decoder = xml.NewDecoder(&clBuffer)
	decoder.CharsetReader = charset.NewReader
	err = decoder.Decode(errors)

	if err != nil {
		// an error was not found!
		//log.Println("^~PAGSEGO~^ Unmarshal(errors)  ERROR: " + err.Error())
		//return
	} else {
		if errors.Errors != nil {
			if len(errors.Errors) > 0 {
				//log.Println("LOL ERRORS")
				//log.Println(errors.Errors[0].Message)
				result.Error = errors
				result.Success = false
				return
			}
		}
	}

	success := &PaymentPreResponse{}

	clBuffer.Truncate(0)
	clBuffer.Write(respBytes)
	decoder = xml.NewDecoder(&clBuffer)
	decoder.CharsetReader = charset.NewReader
	err = decoder.Decode(success)

	if err != nil {
		log.Println("^~PAGSEGO~^ Unmarshal(success)  ERROR: " + err.Error())
		result.Success = false
		return
	}

	result.CheckoutResponse = success
	result.Success = true
	return
}
