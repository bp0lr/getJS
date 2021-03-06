# GetJS
[![License](https://img.shields.io/badge/license-MIT-_red.svg)](https://opensource.org/licenses/MIT)
[![contributions welcome](https://img.shields.io/badge/contributions-welcome-brightgreen.svg?style=flat)](https://github.com/003random/getJS/issues)

getJS is a tool to extract all the javascript files from a set of given urls.  

The urls can also be piped to getJS, or you can specify a singel url with the -url argument. getJS offers a range of options,

varying from completing the urls, to resolving the files.

## Prerequisites

Make sure you have [GO](https://golang.org/) installed on your system.  

### Installing

getJS is written in GO. You can install it with `go get`:

for original code by 003random
```
go get github.com/003random/getJS
```

for my modified code
```
go get github.com/bp0lr/getJS
```

### Why forking

Looks like 003random is not currently merging pull requests.

### tool improvements

* You can add your own headers to the request. (Cookie: foo=bar)
* You can save the files.
* You can save inline code also.

# Usage  
Note: When you supply urls from different sources, e.g. with stdin and an input file, it will add all the urls together :)  
Example: `echo "https://github.com" | getJS -url=https://example.com -input=domains.txt`  

To get all  options, do:  
```bash
getJS -h
```

| Flag | Description | Example |
|------|-------------|---------|
| --url(-u)   | The url to get the javascript sources from | getJS --url=htt<span></span>ps://poc-server.com |
| --header(-h)   | Add custom headers to your request          | getJS --header "Authorization:Bearer token" --header "User-Agent: MyBrowser" |
| --input(-i)   | Input file with urls            | getJS --input=domains.txt |
| --output(-o)   | The file where to save the output to        | getJS --output=output.txt |
| --verbose(-v)  | Display info of what is going on           | getJS --verbose |
| --complete(-c)  | Complete the urls. e.g. /js/index.js -> htt<span></span>ps://example.<span></span>com/js/index.js  | getJS --complete |
| --resolve(-r)   | Resolve the output and filter out the non existing files (Can only be used in combination with -complete)   | getJS --complete --resolve |
| --nocolors(-n)   | Don't color the output   | getJS --nocolors |
| --save(-s)   | Download and save the files to the disk   | getJS --save |


## asciinema
 [![asciicast](https://asciinema.org/a/ebAbqPkLDbDss6CQTMhvf8mZL.png)](https://asciinema.org/a/ebAbqPkLDbDss6CQTMhvf8mZL)


## Examples  

getJS supports stdin data. To pipe urls to getJS, use the following:  

```bash
$ cat domains.txt | getJS
```  

To add custom headers you can use:
```bash
$ getJS -u "https://poc-server.com" -H "Cookie: foo=bar" -H "User-Agent: MyCustomAgent"
```

To save files and inline code to your disk, you can use:  
```bash
$ getJS --url=https://poc-server.com --save
```

If you would like the output to be in JSON format, you can combine it with [@Tomnomnom's](https://github.com/tomnomnom) [toJSON](https://github.com/tomnomnom/hacks/tree/master/tojson):  
```bash
$ getJS --url=https://poc-server.com | tojson
```  

To feed urls from a file use:  
```bash
$ getJS --input=domains.txt
```  

To save the results to a file, and don't display anything, use:  
```bash
$ getJS --url=https://poc-server.com --output=results.txt
```  

If you want to have a list of full urls as output use:  
```bash
$ getJS --url=domains.txt --complete
```  

If you want to only show the existing js files, use:  
```bash
$ getJS --url=domains.txt --complete --resolve
```  

## Built With

* [GO](http://golang.org/) - GOlanguage
* [Goquery](https://github.com/PuerkitoBio/goquery) - HTML parser with syntaxes like jquery, in GO


## Contributing

You are free to submit any issues and/or pull requests :)

## License

This project is licensed under the MIT License.

## Acknowledgments

* [@003random](https://github.com/003random) for the code base.
* [@pczajkowski](https://github.com/pczajkowski) for some nice code improvement and ideas.

---
