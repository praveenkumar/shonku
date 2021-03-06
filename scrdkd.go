package main

import (
	"bufio"
	"bytes"
	"code.google.com/p/go-sqlite/go1/sqlite3"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"github.com/russross/blackfriday"
	"html/template"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
	"flag"
	"encoding/json"
)

type Configuration struct {
	Author 			string
	Title 			string
	URL 			string
	Content_footer	string
	Disqus			string

}

type Article struct {
	Id   int
	Path string
	Hash string
}

type Post struct {
	Title string
	Slug  string
	Body  template.HTML
	Date  time.Time
	Tags  []string
}

type Indexposts struct {
	Posts []Post
	NextF bool
	PreviousF bool
	Next int
	Previous int
	NextLast bool
}


type ByDate []Post

func (a ByDate) Len() int           { return len(a) }
func (a ByDate) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDate) Less(i, j int) bool { return a[i].Date.After(a[j].Date) }

type ByODate []Post

func (a ByODate) Len() int           { return len(a) }
func (a ByODate) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByODate) Less(i, j int) bool { return a[j].Date.After(a[i].Date) }

/*
Creates the empty build database.
*/
func createdb() {
	filename := ".scrdkd.db"
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		// Our db is missing, time to recreate it.
		conn, err := sqlite3.Open(filename)
		if err != nil {
			fmt.Println("Unable to open the database: %s", err)
			os.Exit(1)
		}
		defer conn.Close()
		conn.Exec("CREATE TABLE builds(id INTEGER PRIMARY KEY AUTOINCREMENT, path TEXT, hash TEXT);")
	}
}

/*
Finds all the files from our posts directory.
*/
func findfiles() []string {
	files, _ := ioutil.ReadDir("./posts/")
	names := make([]string, 0)
	for _, f := range files {
		names = append(names, "./posts/"+f.Name())
	}
	return names
}

/*
Creates the hash for each files.
*/
func create_hash(filename string) string {
	md, err := ioutil.ReadFile(filename)
	if err == nil {
		data := sha256.Sum256(md)
		data2 := data[:]
		s := base64.URLEncoding.EncodeToString(data2)
		return s
		//fmt.Println(s)
	}
	return ""
}

/*
Finds out if the file content changed from the last build.
*/
func changed_ornot(filename, hash string) bool {
	db := ".scrdkd.db"
	if _, err := os.Stat(db); err == nil {
		// Our db is missing, time to recreate it.
		conn, err := sql.Open("sqlite3", db)
		if err != nil {
			fmt.Println("Unable to open the database: %s", err)
			os.Exit(1)
		}
		defer conn.Close()
		stmt := fmt.Sprintf("SELECT id, path, hash FROM builds where path='%s';", filename)
		rows, err := conn.Query(stmt)
		defer rows.Close()
		if rows.Next() {
			var article Article
			err = rows.Scan(&article.Id, &article.Path, &article.Hash)
			if article.Hash == hash {
				return false
			} else { // File hash has changed, we need to update the db
				stmt = fmt.Sprintf("UPDATE builds SET hash='%s' where id=%d;", hash, article.Id)
				rows.Close()
				_, err = conn.Exec(stmt)
				if err != nil {
					fmt.Println(err)
				}
				return true
			}
		} else { // Should be insert into DB
			conn.Exec("INSERT INTO builds(path, hash) VALUES (?, ?)", filename, hash)
			return true
		}

	}
	return true
}

/*
Returns the slug of the article
TODO:
We need to improve this function much more.
*/
func get_slug(s string) string {
	s = strings.Replace(s, "(", "-", -1)
	s = strings.Replace(s, ")", "-", -1)
	s = strings.Replace(s, " ", "-", -1)
	s = strings.Replace(s, "(", "-", -1)
	s = url.QueryEscape(s)
	return strings.ToLower(s)
}

/*
Reads a post and gets all details from it.
*/
func read_post(filename string) Post {
	var buffer bytes.Buffer
	var p Post
	flag := false
	f, err := os.Open(filename)
	if err != nil {
		fmt.Println(err)
		return p
	}
	defer f.Close()
	r := bufio.NewReader(f)
	titleline, err := r.ReadString('\n')
	dateline, err := r.ReadString('\n')
	line, err := r.ReadString('\n')
	tagline := line

	for err == nil {
		if line == "====\n" {
			flag = true
			line, err = r.ReadString('\n')
			continue
		}
		if flag {
			buffer.WriteString(line)
		}
		line, err = r.ReadString('\n')
	}

	if err == io.EOF {
		title := titleline[6:]
		title = strings.TrimSpace(title)
		date := dateline[5:]
		date = strings.TrimSpace(date)
		tagsnonstripped := strings.Split(tagline[5:], ",")
		tags := make([]string, 0)
		for i := range tagsnonstripped {
			word := strings.TrimSpace(tagsnonstripped[i])
			tags = append(tags, word)
		}

		p.Title = title
		p.Slug = get_slug(title)
		body := blackfriday.MarkdownCommon(buffer.Bytes())
		p.Body = template.HTML(string(body))
		p.Date = get_time(date)
		p.Tags = tags

	}
	return p
}

func get_time(text string) time.Time {
	const longform = "2006-01-02 15:04:05.999999999 -0700 MST"
	//now := time.Now()
	//x := now.String()
	//fmt.Println(x)
	new_now, err := time.Parse(longform, text)
	if err != nil {
		fmt.Println(err)
	}
	return new_now

}

/*
Builds a post based on the template
*/
func build_post(ps Post) string {
	var doc bytes.Buffer
	var body string
	tml, _ := template.ParseFiles("./templates/post.html")
	err := tml.Execute(&doc, ps)
	if err != nil {
		fmt.Println(err)
	}
	body = doc.String()
	name := "./output/" + ps.Slug + ".html"
	f, err := os.Create(name)
	defer f.Close()
	n, err := io.WriteString(f, body)

	if err != nil {
		fmt.Println(n, err)
	}

	return body
}

/*
Creates index pages.
*/
func build_index(pss []Post, index, pre, next int) {
	fmt.Println(index, pre, next)
	var doc bytes.Buffer
	var body, name string
	var ips Indexposts
	ips.Posts = pss
	if pre != 0 {
		ips.PreviousF = true
		ips.Previous = pre
	} else {
		ips.PreviousF = false
	}
	if next > 0 {
		ips.NextF = true
		ips.Next = next
	} else if next == -1 {
		ips.NextF = false
	} else {
		ips.NextF = true
		ips.Next = next
	}
	if next == 0 {
		ips.NextLast = true
	}

	tml, _ := template.ParseFiles("./templates/index.html")
	err := tml.Execute(&doc, ips)
	if err != nil {
		fmt.Println(err)
	}
	body = doc.String()
	if next == -1 {
		name = "./output/index.html"
	} else {
		name = fmt.Sprintf("./output/index-%d.html", index)
	}
	f, err := os.Create(name)
	defer f.Close()
	n, err := io.WriteString(f, body)

	if err != nil {
		fmt.Println(n, err)
	}
}

/*
Checks for path.
*/
func exists(path string) bool {
    _, err := os.Stat(path)
    if err == nil { return true }
    if os.IsNotExist(err) { return false }
    return false
}

/*
Creates the required directory structure.
*/
func create_dirs(){
	 if !exists("./templates/") {
		os.Mkdir("./templates/", 0777)
	 }
	 if !exists("./output/") {
		os.Mkdir("./output/", 0777)
	 }
	 if !exists("./posts/") {
		os.Mkdir("./posts/", 0777)
	 }

}

/*
Creates new site
*/
func create_site(){

	create_dirs()
}

/*
Reads and returns the configuration.
*/
func get_conf()Configuration{
	file, _ := os.Open("conf.json")
	decoder := json.NewDecoder(file)
	//configuration := make(Configuration)
	var configuration Configuration
	decoder.Decode(&configuration)
	return configuration
}


func main() {

	new_site := flag.Bool("new_site", false, "Creates a new site.")
	flag.Parse()

	if *new_site {
		create_site()
		os.Exit(0)
	}

	conf := get_conf()
	fmt.Println(conf)


	createdb()
	rebuild_index := false
	ps := make([]Post, 0)
	//sort_index := make([]Post, 0)
	names := findfiles()
	for i := range names {
		hash := create_hash(names[i])
		post := read_post(names[i])
		ps = append(ps, post)
		if changed_ornot(names[i], hash) {
			fmt.Println(names[i])
			build_post(post)
			rebuild_index = true

		}
	}

	sort.Sort(ByODate(ps))

	if rebuild_index == true {
		var prev, next int
		index := 1
		num := 0
		length := len(ps)
		sort_index := make([]Post, 0)
		for i := range ps {
			sort_index = append(sort_index, ps[i])
			num = num + 1
			if num == 3 {
				sort.Sort(ByODate(sort_index))
				if index == 1 {
					prev = 0
				} else {
					prev = index -1
				}
				if (index * 3) < length && (length - index*3) > 3 {
					next = index +1
				} else {
					next = 0
				}
				build_index(sort_index, index, prev, next)

				sort_index = make([]Post, 0)
				index = index + 1
				num = 0

			}
		}
		if len(sort_index) > 0 {
			sort.Sort(ByODate(sort_index))
			build_index(sort_index, 0, index-1, -1)
		}
	}

}
