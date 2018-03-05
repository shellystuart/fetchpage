package main

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

type kv struct {
	Key   string
	Value int
}

func main() {
	http.HandleFunc("/fetchpage", fetchPageHandler)
	http.HandleFunc("/submit", inputHandler)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}

func fetchPageHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `<h1>Page Word Count</h1>
		<form action='/submit' method='POST'>
		<input type='text' name='url' label='Enter URL:'></textarea>
		<input type='submit' value='Submit'>
		</form>`)
}

func inputHandler(w http.ResponseWriter, r *http.Request) {
	page := r.PostFormValue("url")
	if _, err := url.ParseRequestURI(page); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Invalid URL submitted")
		return
	}
	var wg sync.WaitGroup
	content := make(chan []string, 2000)

	wg.Add(1)
	go processPage(page, content, &wg, true)

	go func() {
		wg.Wait()
		close(content)
	}()

	type data struct {
		URL   string
		Words []kv
	}

	wordList := template.Must(template.New("wordlist").Parse(`
		<h1>Results for {{.URL}}</h1>
		<table>
		<tr style='text-align: right'>
			<th>Word</th>
			<th>Count</th>
		</tr>
		{{range .Words}}
		<tr style='text-align: right'>
			<td>{{.Key}}</td>
			<td>{{.Value}}</td>
		</tr>
		{{end}}
		</table>
		`))

	wordCount := make(map[string]int)
	for words := range content {
		for _, word := range words {
			if len(word) > 0 {
				wordCount[word]++
			}
		}
	}

	sw := sortWords(wordCount)

	wordList.Execute(w, data{page, sw})
}

func processPage(url string, out chan []string, wg *sync.WaitGroup, first bool) {
	defer wg.Done()
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()
	z := html.NewTokenizerFragment(resp.Body, "body")
	stt := z.Token()
	var links []string
	var words []string

loop:
	for {
		switch tt := z.Next(); tt {
		case html.ErrorToken:
			break loop
		case html.StartTagToken:
			stt = z.Token()
			if stt.Data == "a" && first {
				for _, a := range stt.Attr {
					if a.Key == "href" {
						if !contains(links, a.Val) {
							links = append(links, a.Val)
							//check for valid URL, if relative, add base URL
							fmt.Println("About to process", a.Val)
							wg.Add(1)
							go processPage(a.Val, out, wg, false)
						}
						break
					}
				}
			}
		case html.TextToken:
			if contains([]string{"script", "style"}, stt.Data) {
				continue
			}
			content := strings.TrimSpace(html.UnescapeString(string(z.Text())))
			if len(content) > 0 {
				regSpecChar := regexp.MustCompile(`[^a-zA-Z'\s-]+`)
				content = regSpecChar.ReplaceAllString(content, "")
				regSpaceDash := regexp.MustCompile(`[\s\p{Zs}]{2,}|-|\n+`)
				content = regSpaceDash.ReplaceAllString(content, " ")
				contentSlice := strings.Split(content, " ")
				words = append(words, contentSlice...)
			}
		}
	}
	fmt.Println("Analysis of", url, "complete")
	out <- words
}

func contains(strSlice []string, s string) bool {
	for _, value := range strSlice {
		if value == s {
			return true
		}
	}
	return false
}

func sortWords(words map[string]int) []kv {
	var ss []kv
	for k, v := range words {
		ss = append(ss, kv{k, v})
	}

	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Value > ss[j].Value
	})

	return ss
}
