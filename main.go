package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var filePath string

func main() {
	port := flag.String("p", "8080", "port to listen on")
	file := flag.String("f", "", "path to markdown file (required)")
	flag.Parse()

	if *file == "" {
		fmt.Fprintln(os.Stderr, "Usage: weight-tracker -f <file.md> [-p <port>]")
		flag.PrintDefaults()
		os.Exit(1)
	}
	filePath = *file

	if _, err := os.Stat(filePath); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot open %s: %v\n", filePath, err)
		os.Exit(1)
	}

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/submit", handleSubmit)

	addr := ":" + *port
	fmt.Printf("Listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func readFile() (string, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func extractSVG(content string) string {
	re := regexp.MustCompile(`(?s)<svg[\s\S]*?</svg>`)
	return re.FindString(content)
}

func extractEntries(content string) []string {
	re := regexp.MustCompile(`(?s)<pre>([\s\S]*?)</pre>`)
	m := re.FindStringSubmatch(content)
	if m == nil {
		return nil
	}
	var entries []string
	scanner := bufio.NewScanner(strings.NewReader(strings.TrimSpace(m[1])))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			entries = append(entries, line)
		}
	}
	return entries
}

func todayStr() string {
	return time.Now().Format("2006-01-02")
}

func entryExistsForToday(entries []string) bool {
	today := todayStr()
	for _, e := range entries {
		if strings.HasPrefix(e, today) {
			return true
		}
	}
	return false
}

func updatePreBlock(content, weight string) string {
	today := todayStr()
	newLine := today + "," + weight

	re := regexp.MustCompile(`(?s)(<pre>)([\s\S]*?)(</pre>)`)
	return re.ReplaceAllStringFunc(content, func(block string) string {
		parts := re.FindStringSubmatch(block)
		if parts == nil {
			return block
		}
		raw := parts[2]
		lines := strings.Split(raw, "\n")
		replaced := false
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), today) {
				lines[i] = newLine
				replaced = true
				break
			}
		}
		if !replaced {
			trimmed := strings.TrimRight(raw, "\n ")
			raw = trimmed + "\n" + newLine + "\n"
		} else {
			raw = strings.Join(lines, "\n")
		}
		return parts[1] + raw + parts[3]
	})
}

func handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	weight := strings.TrimSpace(r.FormValue("weight"))
	if weight == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !strings.Contains(weight, ".") {
		weight += ".0"
	}

	content, err := readFile()
	if err != nil {
		http.Error(w, "Could not read file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	updated := updatePreBlock(content, weight)

	if err := os.WriteFile(filePath, []byte(updated), 0644); err != nil {
		http.Error(w, "Could not write file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Run blog-weight to regenerate graph / markdown.
	cmd := exec.Command("blog-weight")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("blog-weight exited with error: %v", err)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	content, err := readFile()
	if err != nil {
		http.Error(w, "Could not read file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	svg := extractSVG(content)
	entries := extractEntries(content)
	today := todayStr()
	alreadyLogged := entryExistsForToday(entries)

	lastEntry := ""
	if len(entries) > 0 {
		lastEntry = entries[len(entries)-1]
	}

	start := len(entries) - 14
	if start < 0 {
		start = 0
	}
	recent := entries[start:]

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	autofocusAttr := "autofocus"

	alreadyLoggedHTML := ""
	if alreadyLogged {
		alreadyLoggedHTML = `<p class="logged">Already logged today: ` + lastEntry + `</p>`
	}

	recentRows := ""
	for _, e := range recent {
		parts := strings.SplitN(e, ",", 2)
		date, val := parts[0], ""
		if len(parts) == 2 {
			val = parts[1]
		}
		recentRows += fmt.Sprintf("<tr><td>%s</td><td>%s kg</td></tr>\n", date, val)
	}

	recentTable := ""
	if len(recent) > 0 {
		recentTable = `<div class="card"><h2>Recent entries</h2><table>` + recentRows + `</table></div>`
	}

	svgBlock := ""
	if svg != "" {
		svgBlock = `<div class="card"><h2>Chart</h2>` + svg + `</div>`
	}

	html := `<!doctype html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
<title>Weight Logger</title>
<style>
	*{box-sizing:border-box;margin:0;padding:0}
	.card h2,body,button,button:hover{color:#d4d4d4}
	.card h2,button{font-size:1rem;color:#d4d4d4}
	.card h2{margin-bottom:12px}
	.card,body{padding:16px}
	.card{background:#222;border-radius:12px;margin-bottom:16px;border:1px solid #333;box-shadow:none}
	.date-label{font-size:.85rem;color:#777;margin-bottom:8px}
	.input-row{display:flex;gap:8px;align-items:center}
	.logged{font-size:.9rem;color:#7cbf7c;font-weight:600;margin-top:8px}
	body{font-family:Consolas,Menlo,'DejaVu Sans Mono',monospace;background:#1a1a1a;max-width:600px;margin:0 auto}
	button:active{opacity:.8}
	button:hover{background:#2e2e2e}
	button{padding:10px 20px;background:#2a2a2a;border:1px solid #333;border-radius:8px;cursor:pointer;white-space:nowrap}
	input[type=number]::-webkit-inner-spin-button,input[type=number]::-webkit-outer-spin-button{-webkit-appearance:none}
	input[type=number]{flex:1;font-size:1.4rem;padding:10px 14px;border:2px solid #333;border-radius:8px;background:#1a1a1a;color:#d4d4d4;-moz-appearance:textfield}
	svg,table{width:100%}
	svg{height:auto;display:block}
	svg text {fill: #d4d4d4}
	table{border-collapse:collapse;font-size:.9rem}
	td:last-child{text-align:right;font-variant-numeric:tabular-nums}
	td{padding:6px 4px;border-bottom:1px solid #333}
	tr:last-child td{border-bottom:none;font-weight:600}
</style>
</head>
<body>
<div class="card">
  <h2>Log today's weight</h2>
  <p class="date-label">` + today + `</p>
  <form method="POST" action="/submit">
    <div class="input-row">
      <input type="number" name="weight" step="0.1" min="30" max="300"
             placeholder="kg" inputmode="decimal" ` + autofocusAttr + `>
      <button type="submit">Save</button>
    </div>
  </form>
  ` + alreadyLoggedHTML + `
</div>
` + recentTable + `
` + svgBlock + `
</body></html>`

	fmt.Fprint(w, html)
}
