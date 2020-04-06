package main

import (
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"gopkg.in/gookit/color.v1"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

// definition is a struct for storing simple word definitions.
type definition struct {
	wordType string // noun, verb, interjection, intransitive verb, etc
	text     string // The actual definition itself
}

// ctxDefinition includes additional info about a definition.
type ctxDefinition struct {
	dict string // The dictionary the definition comes from
	rank uint8  // Where this definition is compared to the others
	def  definition
}

// byDictionary sorts ctxDefintions by rank and dictionary.
// Returns a map with dictionary names as keys, and definition slices as values
func byDictionary(cDs []ctxDefinition) map[string][]definition {
	pre := make(map[string][]ctxDefinition) // Used for ranking, not returned
	// Add all the defintions to the map
	for _, cD := range cDs {
		pre[cD.dict] = append(pre[cD.dict], cD)
	}
	// Sort by rank
	for k := range pre {
		sort.Slice(pre[k], func(i, j int) bool {
			return pre[k][i].rank < pre[k][j].rank
		})
	}
	// Convert to hold definitions only, not context
	m := make(map[string][]definition)
	for dict, cDs := range pre {
		for _, cD := range cDs {
			m[dict] = append(m[dict], cD.def)
		}
	}
	return m
}

// render returns a formatted definition, optionally with color.
// This contains some opinionted color defaults, as opposed to renderOps
func (d *definition) render(c bool) string {
	if c {
		return color.New(color.OpItalic).Render(d.wordType) + "\t" + d.text
	}
	return d.wordType + "\t" + d.text
}

// renderOps returns a formatted color definition, according to the provided styles.
func (d *definition) renderOps(wordType, text color.Style) string {
	return wordType.Render(d.wordType) + "\t\t" + text.Render(d.text)
}

// pprintCtxDefs pretty prints multiple context definitions, optionally with color.
func pprintCtxDefs(cDs []ctxDefinition, c bool) {
	m := byDictionary(cDs)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	//esc := string(tabwriter.Escape)
	for dict, defs := range m {
		if c {
			// Bracket dict name with escape characters so it's not part of the tabbing
			fmt.Fprintln(w, color.New(color.BgGray).Render(dict))
			// Print first definition differently
			fmt.Fprintf(w, "%s\n", defs[0].renderOps(color.New(color.OpItalic, color.OpBold), color.New(color.Cyan)))
			for _, def := range defs[1:] {
				fmt.Fprintf(w, "%s\n", def.render(true))
			}
		} else {
			fmt.Fprintf(w, dict+"\n")
			for _, def := range defs {
				fmt.Fprintf(w, "%s\n", def.render(false))
			}
		}
		fmt.Fprintln(w)
	}
	w.Flush()
}

// wordnikLookup returns a slice of ctxDefinitions for the provided word.
// Looks up words using wordnik.com
func wordnikLookup(w string, client *http.Client) ([]ctxDefinition, error) {
	req, err := http.NewRequest("GET", "https://www.wordnik.com/words/"+w, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/74.0.3729.169 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("couldn't connect to wordnik")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("200 not returned, likely a non-word like '../test' was passed")
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, errors.New("malformed HTML from wordnik")
	}
	ret := make([]ctxDefinition, 0)
	s := doc.Find(".word-module.module-definitions#define .guts.active").First()
	dicts := s.Find("h3")
	lists := s.Find("ul")
	// Go through each list of defs., then each def., and add them
	lists.Each(func(i int, list *goquery.Selection) {
		list.Find("li").Each(func(j int, def *goquery.Selection) {
			// wordType
			wT := def.Find("abbr").First().Text() + " " + def.Find("i").First().Text()
			wT = strings.TrimSpace(wT)
			// dictionary
			d := dicts.Get(i).FirstChild.Data[5:]             // strip the "from " prefix
			d = strings.ToUpper(string(d[0])) + string(d[1:]) // Capitalize first letter
			if string(d[len(d)-1]) == "." {                   // Remove ending period
				d = string(d[:len(d)-1])
			}
			// definition text - remove the wordType at the beginning of the definition
			t := strings.TrimSpace(def.Text()[len(wT):])
			t = strings.ToUpper(string(t[0])) + string(t[1:]) // Capitalize first letter
			ret = append(ret, ctxDefinition{
				dict: d,
				rank: uint8(j),
				def: definition{
					wordType: wT,
					text:     t,
				},
			})
		})
	})
	return ret, nil

}

func main() {
	if len(os.Args) <= 1 {
		fmt.Println("Provide a word to lookup.")
		return
	}
	// TODO: Support multiple words concurrently
	client := &http.Client{}
	words := os.Args[1:]
	// Lookup each word concurrently and store results
	results := make([]chan []ctxDefinition, 0)
	for i, word := range words {
		results = append(results, make(chan []ctxDefinition))
		go func(ind int, w string) {
			defs, err := wordnikLookup(w, client)
			if err != nil {
				panic(err)
			}
			results[ind] <- defs
		}(i, word)
	}

	// Print the answer of each word
	for i, result := range results {
		// TODO: Write to buffer, then flush after result comes in
		color.New(color.BgRed, color.White).Println(words[i])
		pprintCtxDefs(<-result, true)
	}
}
