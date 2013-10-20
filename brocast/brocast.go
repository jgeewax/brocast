package brocast

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/mail"
	"appengine/taskqueue"
	"appengine/user"
)

const (
	GOOGLE_MAPS_BASE_URL = "https://maps.google.com/maps?q="
	BROCAST_EMAIL = "brocastmailer@gmail.com"
)

// Message represents a message sent on Brocast.
// Each Message contains the Lat/Long location of the sender, the sender's
// key, and a message body.
type Message struct {
	GeoLocation string
	Body        string
	Recipients  []string
	Account     string
	Timestamp   time.Time
}

var rootTmpl = template.Must(template.ParseFiles("tmpl/base.html", "tmpl/root.html"))

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if err := rootTmpl.Execute(w, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func brocastHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	u := user.Current(c)
	body, err := ioutil.ReadAll(r.Body)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var message Message
	if err := json.Unmarshal(body, &message); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	message.Account = u.String()
	message.Timestamp = time.Now()
	k := datastore.NewIncompleteKey(c, "Message", nil)
	key, err := datastore.Put(c, k, &message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	t := taskqueue.NewPOSTTask("/mailworker", map[string][]string{"message_key": {key.Encode()}})
	if _, err := taskqueue.Add(c, t, ""); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Response received")

}

func mailWorker(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	// Get the message key from the request
	messageKey := r.FormValue("message_key")
	key, err := datastore.DecodeKey(messageKey)
	if err != nil {
		c.Errorf("%v", err)
	}
	c.Infof("Processing message: %v", messageKey)

	// Retrieve the Message from the datastore
	var message Message
	if err := datastore.Get(c, key, &message); err != nil {
		c.Errorf("%v", err)
		return
	}

	c.Infof("Sending mail for message: %v", messageKey)

	// Send an email to each recipient with a google maps link
	mapsurl := fmt.Sprintf("%v%v", GOOGLE_MAPS_BASE_URL, message.GeoLocation)
	body := fmt.Sprintf("%v\n\nLocation: %v", message.Body, mapsurl)
	msg := &mail.Message{
		Sender:  fmt.Sprintf("Brocast <%v>", BROCAST_EMAIL),
		To:      message.Recipients,
		Subject: fmt.Sprintf("Brocast from %v", message.Account),
		Body:    body,
	}
	if err := mail.Send(c, msg); err != nil {
		c.Errorf("%v", err)
		return
	}

	c.Infof("Mail sent for message: %v", messageKey)
}

func init() {
	http.HandleFunc("/brocasts", brocastHandler)
	http.HandleFunc("/mailworker", mailWorker)
	http.HandleFunc("/", rootHandler)
}
