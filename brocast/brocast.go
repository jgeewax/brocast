package brocast

import (
	"bytes"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"io/ioutil"
	"net/http"
	texttemplate "text/template"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/mail"
	"appengine/taskqueue"
	"appengine/user"
)

const (
	GOOGLE_MAPS_BASE_URL = "https://maps.google.com/maps?q="
	BROCAST_EMAIL        = "brocastmailer@gmail.com"
)

// Message represents a message sent on Brocast.
// Each Message contains the Lat/Long location of the sender, the sender's
// key, and a message body.
type Message struct {
	GeoLocation string
	Body        string
	Recipients  []string
	Account     string
	Sender      string
	Timestamp   time.Time
}

type emailCtx struct {
	Body       string
	MapsURL string
}

var rootTmpl = htmltemplate.Must(htmltemplate.ParseFiles("tmpl/base.html", "tmpl/root.html"))
var emailTemplate = texttemplate.Must(texttemplate.New("email").Parse(emailText))

const emailText = `I'm at {{.MapsURL}}

{{.Body}}

Send your own Brocasts at http://brocast.appspot.com/`

func rootHandler(resp http.ResponseWriter, req *http.Request) {
	if err := rootTmpl.Execute(resp, nil); err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
	}
}

func brocastHandler(resp http.ResponseWriter, req *http.Request) {
	ctx := appengine.NewContext(req)

	// We don't need to check this for nil since /.* is login: required.
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return
	}

	var message Message
	if err := json.Unmarshal(body, &message); err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return
	}

	message.Account = user.Current(ctx).String()
	message.Timestamp = time.Now()
	k := datastore.NewIncompleteKey(ctx, "Message", nil)
	key, err := datastore.Put(ctx, k, &message)
	if err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return
	}

	keyString := key.Encode()
	ctx.Infof(fmt.Sprintf("Dispatching task to process message: %v", keyString))
	task := taskqueue.NewPOSTTask("/mailworker", map[string][]string{"message_key": {keyString}})
	if _, err := taskqueue.Add(ctx, task, ""); err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return
	}

	resp.WriteHeader(http.StatusCreated)
	return

}


func mailWorker(resp http.ResponseWriter, req *http.Request) {
	ctx := appengine.NewContext(req)

	messageKey := req.FormValue("message_key")
	key, err := datastore.DecodeKey(messageKey)
	if err != nil {
		ctx.Errorf("%v", err)
	}
	ctx.Infof("Processing message: %v", messageKey)

	// Retrieve the Message from the datastore
	var message Message
	if err := datastore.Get(ctx, key, &message); err != nil {
		ctx.Errorf("%v", err)
		return
	}

	ctx.Infof("Sending mail for message: %v", messageKey)

	// Send an email to each recipient with a google maps link
	var buffer bytes.Buffer
	err = emailTemplate.Execute(&buffer, emailCtx{
		MapsURL: fmt.Sprintf("%v%v", GOOGLE_MAPS_BASE_URL, message.GeoLocation),
		Body:    message.Body,
	})

	if err != nil {
		ctx.Errorf("%v", err)
		return
	}

	body := buffer.String()
	msg := &mail.Message{
		Sender:  fmt.Sprintf("%v <%v>", message.Sender, BROCAST_EMAIL),
		To:      message.Recipients,
		Subject: fmt.Sprintf("Brocast from %v", message.Sender),
		Body:    body,
	}
	if err := mail.Send(ctx, msg); err != nil {
		ctx.Errorf("%v", err)
		return
	}

	ctx.Infof("Mail sent for message: %v", messageKey)
}

func init() {
	http.HandleFunc("/brocasts", brocastHandler)
	http.HandleFunc("/mailworker", mailWorker)
	http.HandleFunc("/", rootHandler)
}
