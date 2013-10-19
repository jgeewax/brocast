package brocast

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"

	"appengine"
	"appengine/datastore"
	"appengine/user"
)

// Message represents a message sent on Brocast.
// Each Message contains the Lat/Long location of the sender, the sender's
// key, and a message body.
type Message struct {
	GeoLocation string
	Body        string
	Recipients  []string
	Account     string
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
	key := datastore.NewIncompleteKey(c, "Message", nil)
	if _, err := datastore.Put(c, key, &message); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Response received")

}

func init() {
	http.HandleFunc("/brocasts", brocastHandler)
	http.HandleFunc("/", rootHandler)
}
