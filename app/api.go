package main
import (
    "code.google.com/p/gorest"
        "net/http"
	"github.com/gographics/imagick/imagick"
	"io/ioutil"
	"strconv"
	"fmt"
	"os"
	"log"
        "encoding/json"
 	"path/filepath"
	"mime"
)


var transformationQueue = make(chan *AssetProcessingRequest, 1)
var fetchQueue = make(chan *AssetProcessingRequest, 1)

func main() {


    origin := os.Getenv("ORIGIN_SERVER")	
    if origin == "" {	
 	panic("You need to configure ORIGIN_SERVER environement variable")	
    }

    imagick.Initialize()
    // Schedule cleanup
    defer imagick.Terminate()
    go HandleTransform(transformationQueue)   

    const NCPU = 10 
     for i := 0; i < NCPU; i++ {
        go HandleFetch(fetchQueue) 
    }	 
	
    gorest.RegisterService(new(ImageService)) 
    //Register our service
    http.Handle("/",gorest.Handle())    
    http.ListenAndServe(":8787",nil)
}

//Service Definition
type ImageService struct {
    gorest.RestService `root:"/image/"`
    logo    gorest.EndPoint `method:"GET" path:"/logo/{w:int}/{h:int}" output:"string"`
    info    gorest.EndPoint `method:"GET" path:"/info/{image:string}" output:"string"`
    image    gorest.EndPoint `method:"GET" path:"/resize/{...:string}" output:"string"`
}

type AssetProcessingRequest struct {
    source string	
    dest string 
    width uint
    height uint
    resultChan  chan int	
}

type Asset struct {
    body []byte      
    mime string
    width uint
    height uint
}


func HandleTransform(queue chan *AssetProcessingRequest) {
    for req := range queue {
        code := Resize(req)
        req.resultChan <- code 
    }
}

func HandleFetch(queue chan *AssetProcessingRequest) {
    for req := range queue {
        code := Fetch(req)
        req.resultChan <- code 
    }
}



func(serv ImageService) Logo(w int, h int) string {

	width := uint(w)
	height := uint(h)
	img := "logo.png"
	source := fmt.Sprintf("/var/run/images/%d-%d-%s", 200, 200, img)
	dest := fmt.Sprintf("/var/run/images/%d-%d-%s", w, h, img)
	
	contents, err := ioutil.ReadFile(dest)
	
	asset := &Asset{ contents, mime.TypeByExtension(filepath.Ext(source)), width, height}
        if err != nil {
		log.Printf("File not found %s %s", dest, err) 
		r := &AssetProcessingRequest{source, dest, width, height, make(chan int)}
		
		log.Printf("Queuing request %s", r.dest) 
		transformationQueue <- r
		// Wait for response
		log.Printf("Processed %d", <-r.resultChan) 
		contents, err = ioutil.ReadFile(r.dest)	
        	if err != nil {
               		panic("ReadFile error expected, none found " + r.dest)
        	}
		asset = &Asset{ contents,  mime.TypeByExtension(filepath.Ext(source)),width, height}
	}
	
	serv.ResponseBuilder().SetContentType(asset.mime)
	serv.ResponseBuilder().CacheMaxAge(60*60*24) 
	return string(asset.body) 
}

func(serv ImageService) Image(v... string) string{
	
	// todo better params handling - 
	w, _ := strconv.Atoi(v[0]);
	h, _ := strconv.Atoi(v[1]);
	v = v [2:len(v)]
	width := uint(w)
	height := uint(h)	
	path := ""
	key  := ""
        for _, p := range v {
		path += "/"
               	path += p
		key += "-"
		key += p
        }	
	log.Printf("path %s", path)
	log.Printf("key %s", key)

    	origin := os.Getenv("ORIGIN_SERVER")	
	remote  := fmt.Sprintf("%s%s", origin, path)
	source := fmt.Sprintf("/var/run/images/%s", key)
	dest := fmt.Sprintf("/var/run/images/%d-%d-%s", w, h, key)

	contents, err := ioutil.ReadFile(dest)
        asset := &Asset{ contents,  mime.TypeByExtension(filepath.Ext(dest)),width, height}
        if err != nil {
            log.Printf("File not found %s %s", dest, err)
	    	
	    rf := &AssetProcessingRequest{remote, source, 0, 0, make(chan int)}
	    log.Printf("Queuing fetch request %s %s", rf.source, rf.dest)
	    fetchQueue <- rf
	    fetch := <-rf.resultChan
	    log.Printf("Fetched %d", fetch) 
	    if (fetch == 404) {
	    	   source = fmt.Sprintf("/var/run/images/%s", "notfound.png")
	    } 
	    rt := &AssetProcessingRequest{source, dest, width, height, make(chan int)}
	    transformationQueue <- rt
	    log.Printf("Processed %d", <-rt.resultChan) 
	
	    contents, err := ioutil.ReadFile(rt.dest)
	    asset.mime = mime.TypeByExtension(filepath.Ext(rt.dest))
            asset.body = contents
       	    if err != nil {
           	panic("ReadFile error expected, none found " + rt.dest)
       	    }
	}

	serv.ResponseBuilder().SetContentType(asset.mime)
	serv.ResponseBuilder().CacheMaxAge(60*60*24) 
	return string(asset.body)
	}

	func Fetch(r *AssetProcessingRequest) int {
		log.Printf("Fetch request %s", r.source) 
                var err error
		_, err = ioutil.ReadFile(r.dest)
        	if err == nil {
			log.Printf("304 File already exist : %s", r.dest) 
			return 304 
		}
		log.Printf("Fetching  %s", r.dest) 
	        resp, err := http.Get(r.source)
		if err != nil || resp.StatusCode != 200{
			log.Printf("%d Fetching error : %s", resp.StatusCode, r.dest) 
                        return 404 
                }
		body, err := ioutil.ReadAll(resp.Body)		
		log.Printf("Creating tmp file") 
                tmpFile, err := ioutil.TempFile("", r.dest)
		if err != nil {
                        panic(err)
                }
		log.Printf("Writing to %s tmp file", tmpFile.Name()) 
		tmpFile.Write(body)
                if err != nil {
                        panic(err)
                }
		log.Printf("Rename to %s ", r.dest) 
                os.Rename(tmpFile.Name(), r.dest)
		return 200
	}

	func Resize(r *AssetProcessingRequest) int {
		log.Printf("Resize request %s", r.dest) 
                var err error
		_, err = ioutil.ReadFile(r.dest)
        	if err == nil {
			// file already exists
			//log.Printf("304 File already exist : %s", r.dest) 
			return 304 
		}

                mw := imagick.NewMagickWand()
                // Schedule cleanup
                defer mw.Destroy()
	        
		log.Printf("Loading original file  : %s", r.source) 
                err = mw.ReadImage(r.source)
                if err != nil {
                        panic(err)
                }
		log.Printf("Resizing to %d x %d", r.width, r.height) 
                // Resize the image using the Lanczos filter
		
                // The blur factor is a float, where > 1 is blurry, < 1 is sharp
		if (r.height == 0) {
			originalwidth := mw.GetImageWidth()
			originalheight := mw.GetImageHeight()
			r.height = r.width * originalheight / originalwidth 
		}
		if (r.width == 0) {
                        originalwidth := mw.GetImageWidth()
                        originalheight := mw.GetImageHeight()
                        r.width = r.height * originalwidth / originalheight 
                }

                err = mw.ResizeImage(r.width, r.height, imagick.FILTER_LANCZOS, 1)
                if err != nil {
                        panic(err)
                }      
		log.Printf("Resized to %d x %d", r.width, r.height) 
                // Set the compression quality to 95 (high quality = low compression)
                err = mw.SetImageCompressionQuality(95)
                if err != nil {
                        panic(err)
                }
		
		log.Printf("Creating tmp file") 
                tmpFile, err := ioutil.TempFile("", r.dest)
		if err != nil {
                        panic(err)
                }
		log.Printf("Writing to %s tmp file", tmpFile.Name()) 
                err = mw.WriteImage(tmpFile.Name())

                if err != nil {
                        panic(err)
                }
		
		log.Printf("Rename to %s ", r.dest) 
                os.Rename(tmpFile.Name(), r.dest)
	
		return 200
	}



func(serv ImageService) Info(filename string) string{
	imagick.Initialize()
	// Schedule cleanup
	defer imagick.Terminate()
	var err error

	mw := imagick.NewMagickWand()
	// Schedule cleanup
	defer mw.Destroy()

	err = mw.ReadImage("/var/run/images/logo-200-200.png")
	if err != nil {
		panic(err)
	}
	//properties := mw.GetImageProperties("*")
	properties := mw.GetImageFormat();
	//return properties
	j,err := json.Marshal(properties)
	if err != nil {
                panic(err)
        }
	return string(j) 
}

