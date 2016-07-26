package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/chrisport/go-lang-detector/langdet"
	"github.com/kljensen/snowball"
	"golang.org/x/net/html"
	"path"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"unicode"
)

var langs = []langdet.Language{}
var detector = langdet.Detector{&langs, 0.2}

func init() {
	s := os.Getenv("GOPATH")
	if s == "" {
		log.Fatalf("could not get GOPATH variable")
	}

	p := path.Join(s, "src/github.com/chrisport/go-lang-detector/default_languages.json")
	analyzedInput, err := ioutil.ReadFile(p)
	if err != nil {
		fmt.Println("go-lang-detector/langdet: No default languages loaded. default_languages.json not present")
		return
	}

	err = json.Unmarshal(analyzedInput, &langs)
	if err != nil {
		fmt.Println("go-lang-detector/langdet: Could not unmarshall default languages from default_languages.json")
		return
	}
}

func Language(data string) (string, error) {
	r := detector.GetLanguages(data)
	if len(r) == 0 {
		return "", fmt.Errorf("undefined language")
	}

	return r[0].Name, nil
}

func Stem(word string) string {
	language, err := Language(word)
	if err != nil {
		return word
	}

	stemmed, err := snowball.Stem(word, language, true)
	if err != nil {
		return word
	}

	return stemmed
}

// isSeparator reports whether the rune could mark a word boundary.
// TODO: update when package unicode captures more of the properties.
func isSeparator(r rune) bool {
	// ASCII alphanumerics and underscore are not separators
	if r <= 0x7F {
		switch {
		case '0' <= r && r <= '9':
			return false
		case 'a' <= r && r <= 'z':
			return false
		case 'A' <= r && r <= 'Z':
			return false
		case r == '_':
			return false
		}
		return true
	}
	// Letters and digits are not separators
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return false
	}
	// Otherwise, all we can do for now is treat spaces as separators.
	return unicode.IsSpace(r)
}

func StemWords(words []string) ([]string, error) {
	ret := make([]string, 0)
	for _, word := range words {
		ret = append(ret, Stem(word))
	}
	return ret, nil
}

func SplitString(text string) []string {
	return strings.FieldsFunc(text, isSeparator)
}

type Content struct {
	PlainText	[]string
	StemmedText	[]string
	Links		[]string
	Images		[]string
}

func GetAttrKey(z *html.Tokenizer, c *Content, moreAttr bool, lookup string) string {
	lookup_key := []byte(lookup)
	for moreAttr {
		var key, val []byte
		key, val, moreAttr = z.TagAttr()

		if bytes.Equal(key, lookup_key) {
			return string(val)
		}
	}

	return ""
}

func GetAttrs(z *html.Tokenizer, c *Content) {
	name, moreAttr := z.TagName()
	if len(name) == 1 && name[0] == 'a' {
		link := GetAttrKey(z, c, moreAttr, "href")
		if link != "" {
			c.Links = append(c.Links, link)
		}
	}
	if len(name) == 3 && bytes.Equal(name, []byte("img")) {
		link := GetAttrKey(z, c, moreAttr, "src")
		if link != "" {
			c.Images = append(c.Images, link)
		}
	}
}

func Parse(reader io.Reader) (*Content, error) {
	tokenizer := html.NewTokenizer(reader)

	c := &Content {
		PlainText:	make([]string, 0),
		StemmedText:	make([]string, 0),
		Links:		make([]string, 0),
		Images:		make([]string, 0),
	}

	for {
		token := tokenizer.Next()

		switch {
		case token == html.ErrorToken:
			if tokenizer.Err() == io.EOF {
				return c, nil
			}

			return nil, tokenizer.Err()
		case token == html.StartTagToken || token == html.EndTagToken:
			GetAttrs(tokenizer, c)
		case token == html.TextToken:
			text := strings.ToLower(string(tokenizer.Text()))
			words := SplitString(text)
			c.PlainText = append(c.PlainText, words...)

			s, err := StemWords(words)
			if err != nil {
				return c, err
			}

			c.StemmedText = append(c.StemmedText, s...)
		}
	}

	return c, nil
}
