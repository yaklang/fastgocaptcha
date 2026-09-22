// Run with go run ./example/text, then open http://localhost:8127.
package main

import (
	"encoding/json"
	"github.com/yaklang/fastgocaptcha"
	"log"
	"net/http"
	"time"
)

func main() {
	store, err := fastgocaptcha.NewAnswerStore(5*time.Minute, 1024)
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/captcha", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		var data *fastgocaptcha.Data
		var err error
		if r.URL.Query().Get("kind") == "arithmetic" {
			data, err = fastgocaptcha.NewArithmetic(180, 60)
		} else {
			data, err = fastgocaptcha.New(180, 60)
		}
		if err != nil {
			http.Error(w, "generation failed", 500)
			return
		}
		image, err := data.Base64()
		if err != nil {
			http.Error(w, "encoding failed", 500)
			return
		}
		id, err := store.Put(data.Text, true)
		if err != nil {
			http.Error(w, "try later", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]string{"id": id, "image": image})
	})
	http.HandleFunc("/verify", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": store.Verify(r.PostForm.Get("id"), r.PostForm.Get("answer"))})
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><title>FastGoCaptcha text and arithmetic</title>
<h1>Text and arithmetic CAPTCHA</h1><select id="kind"><option value="text">Text</option><option value="arithmetic">Arithmetic</option></select>
<button id="refresh">New challenge</button><p><img id="image" alt="CAPTCHA"></p>
<form id="form"><input name="answer" autocomplete="off" required><button>Verify</button></form><p id="result"></p>
<script>
let id=''; async function refresh(){const d=await(await fetch('/captcha?kind='+document.querySelector('#kind').value)).json();id=d.id;document.querySelector('#image').src=d.image;}
document.querySelector('#refresh').onclick=refresh;document.querySelector('#kind').onchange=refresh;
document.querySelector('#form').onsubmit=async e=>{e.preventDefault();const body=new URLSearchParams(new FormData(e.target));body.set('id',id);const d=await(await fetch('/verify',{method:'POST',body})).json();document.querySelector('#result').textContent=d.success?'Verified':'Incorrect or expired';await refresh();};refresh();
</script>`))
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:8127", nil))
}
