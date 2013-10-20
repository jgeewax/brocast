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
type Brocast struct {
	GeoLocation string
	Body        string
	Recipients  []string
	Account     string
	Sender      string
	Timestamp   time.Time
}

type emailCtx struct {
	Body    string
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

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return
	}

	var brocast Brocast
	if err := json.Unmarshal(body, &brocast); err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return
	}

	// We don't need to check user.Current(ctx) for nil since /.* is login: required.
	brocast.Account = user.Current(ctx).String()
	brocast.Timestamp = time.Now()
	k := datastore.NewIncompleteKey(ctx, "Brocast", nil)
	key, err := datastore.Put(ctx, k, &brocast)
	if err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return
	}

	keyString := key.Encode()
	ctx.Infof(fmt.Sprintf("Dispatching task to process brocast: %v", keyString))
	task := taskqueue.NewPOSTTask("/mailworker", map[string][]string{"brocast_key": {keyString}})
	if _, err := taskqueue.Add(ctx, task, ""); err != nil {
		http.Error(resp, err.Error(), http.StatusInternalServerError)
		return
	}

	resp.WriteHeader(http.StatusCreated)
	return

}

func mailWorker(resp http.ResponseWriter, req *http.Request) {
	ctx := appengine.NewContext(req)

	brocastKey := req.FormValue("brocast_key")
	key, err := datastore.DecodeKey(brocastKey)
	if err != nil {
		ctx.Errorf("%v", err)
	}
	ctx.Infof("Processing brocast: %v", brocastKey)

	// Retrieve the Brocast from the datastore
	var brocast Brocast
	if err := datastore.Get(ctx, key, &brocast); err != nil {
		ctx.Errorf("%v", err)
		return
	}

	ctx.Infof("Sending mail for message: %v", brocastKey)

	// Send an email to each recipient with a google maps link
	var buffer bytes.Buffer
	err = emailTemplate.Execute(&buffer, emailCtx{
		MapsURL: fmt.Sprintf("%v%v", GOOGLE_MAPS_BASE_URL, brocast.GeoLocation),
		Body:    brocast.Body,
	})

	if err != nil {
		ctx.Errorf("%v", err)
		return
	}

	body := buffer.String()
	msg := &mail.Message{
		Sender:  fmt.Sprintf("%v <%v>", brocast.Sender, BROCAST_EMAIL),
		To:      brocast.Recipients,
		Subject: fmt.Sprintf("Brocast from %v", brocast.Sender),
		Body:    body,
	}
	if err := mail.Send(ctx, msg); err != nil {
		ctx.Errorf("%v", err)
		return
	}

	ctx.Infof("Mail sent for brocast: %v", brocastKey)
}

func init() {
	http.HandleFunc("/brocasts", brocastHandler)
	http.HandleFunc("/mailworker", mailWorker)
	http.HandleFunc("/", rootHandler)
}
