package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"crypto/tls"
	"net"

	"github.com/PuerkitoBio/goquery"
	"github.com/logrusorgru/aurora"

	flag "github.com/spf13/pflag"
)

type logger interface {
	Log(msg string)
	Error(msg string, err error)
}

type silent struct{}

func (s silent) Log(msg string) {
}

func (s silent) Error(msg string, err error) {
}

type verbose struct {
}

func (v verbose) Log(msg string) {
	fmt.Println(au.Cyan(msg))
}

func Log(l logger, msg string) {
	l.Log(msg)
}

func (v verbose) Error(msg string, err error) {
	fmt.Fprintln(os.Stderr, au.Red(msg))
	if err != nil {
		fmt.Fprintln(os.Stderr, au.Red("[!] Error: "), au.Red(err))
	}
}

func Error(l logger, msg string, err error) {
	l.Error(msg, err)
}



var output logger
var au aurora.Aurora

var (
	workersArg	      int
	timeOutArg		  int
	HeaderArg         []string
	urlArg            string
	statusListArg     string
	proxyArg          string
	fingerPrintArg    string
	outputFileArg     string
	queryArg          string
	inputFileArg	  string
	verboseArg        bool
	followRedirectArg bool
	useRandomAgentArg bool
	testHTTPArg       bool
	allInArg		  bool
	resolveArg		  bool
	completeArg		  bool
	noColorsArg		  bool
	saveArg			  bool
)


func main() {
	flag.StringVarP(&urlArg, "url", "u", "", "The url to get the javascript sources from")
	flag.StringVarP(&outputFileArg, "output", "o", "", "Output file to save the results to")
	flag.StringVarP(&inputFileArg, "input", "i", "", "Input file with urls")
	flag.StringVarP(&proxyArg, "proxy", "p", "", "Add a HTTP proxy")	
	flag.StringArrayVarP(&HeaderArg, "header", "H", nil, "Any HTTP headers(-H \"Authorization:Bearer token\")")
	flag.BoolVarP(&resolveArg, "resolve", "r", true, "Output only existing files")
	flag.BoolVarP(&completeArg, "complete", "c", true, "Complete the url. e.g. append the domain to the path")
	flag.BoolVarP(&verboseArg, "verbose", "v", false, "Display info of what is going on")
	flag.BoolVarP(&noColorsArg, "nocolors", "n", false, "Enable or disable colors")
	flag.BoolVarP(&saveArg, "save", "s", false, "Save the scripts to files")
	flag.BoolVarP(&followRedirectArg, "follow-redirect", "f", false, "Follow redirects (Default: false)")
	
	flag.Parse()

	au = aurora.NewAurora(!noColorsArg)

	var urls []string
	var allSources []string

	output = silent{}

	if verboseArg {
		output = verbose{}
	}

	stat, err := os.Stdin.Stat()
	if err != nil {
		output.Error("[!] Couldnt read Stdin", err)
	}

	if (stat.Mode() & os.ModeCharDevice) == 0 {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			urls = append(urls, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			output.Error("[!] Couldnt read Stdin", err)
		}
		if len(urls) > 0 {
			output.Log("[+] Received urls from Stdin")
		}
	}

	if inputFileArg != "" {
		lines, err := readLines(inputFileArg)
		if err != nil {
			output.Error("[!] Couldn't read from input file", err)
		}
		output.Log("[+] Set url file to " + inputFileArg)
		urls = append(urls, lines...)
	}

	if urlArg != "" {
		output.Log("[+] Set url to " + urlArg)
		urls = append(urls, urlArg)
	}

	if len(urls) == 0 {
		output.Error("[!] No urls supplied", nil)
		os.Exit(3)
	}

	if resolveArg && !completeArg {
		output.Error("[!] Resolve can only be used in combination with -complete", nil)
		os.Exit(3)
	}

	DomainFolder, err := url.Parse(urlArg)

	for _, e := range urls {
		var sourcesBak []string
		var completedSuccessfully = true
		output.Log("[+] Getting sources from " + e)

		sources, err := getScriptSrc(e, HeaderArg, saveArg, DomainFolder.Host)

		if err != nil {
			output.Error("[!] Couldn't get sources from "+e, err)
		}

		if completeArg {
			output.Log("[+] Completing URLs")
			sourcesBak = sources
			sources, err = completeUrls(sources, e)

			if err != nil {
				output.Error("[!] Couldn't complete URLs", err)
				sources = sourcesBak
				completedSuccessfully = false
			}
		}

		if resolveArg && completeArg {
			if completedSuccessfully {
				output.Log("[+] Resolving files")
				sourcesBak = sources
				sources, err = resolveUrls(sources, saveArg, DomainFolder.Host)
				if err != nil {
					output.Error("[!] Couldn't resolve URLs", err)
					sources = sourcesBak
				}
			} else {
				output.Error("[!] Couldn't resolve URLs", nil)
			}
		} else if resolveArg {
			output.Error("[!] Resolve can only be used in combination with -complete", nil)

		}

		for _, i := range sources {
			fmt.Println(i)
		}

		if outputFileArg != "" {
			allSources = append(allSources, sources...)
		}
	}

	// Save to file
	if outputFileArg != "" {
		output.Log("[+] Saving output to " + outputFileArg)
		err := saveToFile(allSources, outputFileArg)
		if err != nil {
			output.Error("[!] Couldn't save to output file "+outputFileArg, err)
		}
	}

}

func saveJS(link string, data io.Reader, SaveFolder string) {

	u, err := url.Parse(link)
	if err != nil {
		output.Error("[!] Couldn't parse URL", err)
		return
	}

	//check if the download folder exist.
	folderPath, err := os.Getwd()
	folderPath = filepath.Join(folderPath, "download")
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		os.Mkdir(folderPath, os.ModePerm)
	}

	folderPath = filepath.Join(folderPath, SaveFolder)

	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		os.Mkdir(folderPath, os.ModePerm)
	}

	//Now, lest check the file
	filename := filepath.Base(u.Path)
	ext := filepath.Ext(filename)

	if ext == "" {
		ext = ".js"

		if len(filename) > 0 {
			filename = filename + ext
		} else {
			filename = "inline" + ext
		}
	}

	var fullPath = filepath.Join(folderPath, filename)

	_, err = os.Stat(fullPath)
	i := 1
	for err == nil && i < 99 {
		filename = strings.TrimSuffix(filename, ext) + strconv.Itoa(i) + ext
		fullPath = filepath.Join(folderPath, filename)
		//output.Log("[+] newname: " + filename)
		_, err = os.Stat(fullPath)
		i++
	}

	fullPath = filepath.Join(folderPath, filename)

	output.Log("[+] Downloading: " + link + " => " + fullPath)

	out, err := os.Create(fullPath)
	if err != nil {
		output.Error("[!] Couldn't create the file", err)
		return
	}

	_, err = io.Copy(out, data)
	if err != nil {
		output.Error("[!] Problem saving to file", err)
	}

	out.Close()
}

// ToDO: Use channel instead of slide, and use io.Writer instead of file path
func saveToFile(sources []string, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, line := range sources {
		fmt.Fprintln(w, line)
	}
	return w.Flush()
}

func getScriptSrc(url string, headers []string, saveArg bool, SaveFolder string) ([]string, error) {

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	for _, d := range headers {
		values := strings.Split(d, ":")
		if len(values) == 2 {
			output.Log("[+] New Header: " + values[0] + ": " + values[1])
			req.Header.Set(values[0], values[1])
		}
	}

	client := newClient()
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		output.Error("[!] "+url+" returned an "+strconv.Itoa(res.StatusCode)+" instead of an 200 OK", nil)
		return nil, nil
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	var sources []string
	attributes := []string{"src", "data-src"}

	// Find the script tags, and get the src
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		for _, attr := range attributes {
			value, _ := s.Attr(attr)
			strings.TrimSpace(s.Text())
			if value != "" {
				sources = append(sources, value)
				break
			} else if len(s.Text()) > 0 && saveArg {
				saveJS("inline", strings.NewReader(s.Text()), SaveFolder)
				break
			}
		}
	})

	return sources, nil
}

// ToDo: Use io.Writer instead of a file path
func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func resolveUrls(s []string, saveArg bool, SaveFolder string) ([]string, error) {

	var validLinks []string
	for _,link:= range s{	
		
		req, err := http.NewRequest("GET", link, nil)
		if err != nil {
			output.Error("[-] Couldn't generate request URL", err)
			continue
		}
		
		client := newClient()
		resp, err := client.Do(req)
		
		if err != nil {
			output.Error("[!] Couldn't resolve URL", err)
			continue
		}

		if resp != nil {
			output.Log("[==>] " + link + "[" + strconv.Itoa(resp.StatusCode) + "]")
		
			if (resp.StatusCode == 200 || resp.StatusCode == 304){
				validLinks = append(validLinks, link)
				if saveArg {
					saveJS(link, resp.Body, SaveFolder)
				} 
			}
			resp.Body.Close()
		}
	}

	return validLinks, nil
}

func completeUrls(s []string, mainUrl string) ([]string, error) {
	u, err := url.Parse(mainUrl)
	if err != nil {
		return s, err
	}

	for i := range s {
		if strings.HasPrefix(s[i], "//") {
			s[i] = u.Scheme + ":" + s[i]
		} else if strings.HasPrefix(s[i], "/") && string(s[i][1]) != "/" {
			s[i] = u.Scheme + "://" + u.Host + s[i]
		} else if !strings.HasPrefix(s[i], "http://") && !strings.HasPrefix(s[i], "https://") {
			s[i] = u.Scheme + "://" + u.Host + u.Path + "/" + s[i]
		}
	}
	return s, nil
}


func newClient() *http.Client {
	tr := &http.Transport{
		MaxIdleConns:    30,
		IdleConnTimeout: time.Second,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout: time.Second * time.Duration(timeOutArg),
		}).DialContext,
	}

	if proxyArg != "" {
		if p, err := url.Parse(proxyArg); err == nil {
			tr.Proxy = http.ProxyURL(p)
		}
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Second * time.Duration(timeOutArg),
	}

	if !followRedirectArg {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client
}