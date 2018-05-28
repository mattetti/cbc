package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/mattetti/m3u8Grabber/m3u8"

	"github.com/PuerkitoBio/goquery"
)

var (
	Debug bool
)

const (
	presentationURL = "https://ici.tou.tv/presentation/"
)

func main() {
	if len(os.Args) < 2 {
		log.Println("You need to pass the url of the show to download")
		os.Exit(1)
	}

	// os.Args[1]
	passedURL := os.Args[1] // example: "https://ici.radio-canada.ca/jeunesse/scolaire/emissions/1080/mouss-boubidi/episodes/367664/hulla-hop-hop-hop/emission"
	urls, err := listRCCEpisodesFromURL(passedURL)
	if err != nil {
		log.Fatalf("Something went wrong when fetching the URL - %v", err)
	}

	w := &sync.WaitGroup{}
	stopChan := make(chan bool)
	m3u8.LaunchWorkers(w, stopChan)

	outputFileName := "test"

	m3u8.Debug = true
	var url string
	for _, u := range urls {
		if url, err = downloadRCCShowURL(u); err != nil {
			log.Printf("Failed to download %s - %v\n", u, err)
		}
		fmt.Println("->", url)
		job := &m3u8.WJob{
			Type:          m3u8.ListDL,
			URL:           url,
			SkipConverter: false,
			DestPath:      ".",
			Filename:      outputFileName}
		m3u8.DlChan <- job
		// TODO: set filename and not break
		break
	}
	time.Sleep(1 * time.Minute)
	close(m3u8.DlChan)
	w.Wait()
}

func downloadRCCShowURL(u string) (string, error) {
	if Debug {
		log.Printf("Downloading show from %s\n", u)
	}
	res, err := http.Get(u)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", err
	}
	s := doc.Find(".audio-video-console").First()
	val, _ := s.Attr("data-console-info")

	var data RCCEpisodeJSON
	if err = json.Unmarshal([]byte(val), &data); err != nil {
		return "", err
	}

	return rccMediaURL(data.IDMedia)
}

func rccMediaURL(id string) (string, error) {
	url := fmt.Sprintf("https://api.radio-canada.ca/validationMedia/v1/Validation.html?connectionType=broadband&output=json&multibitrate=true&deviceType=ipad&appCode=medianet&idMedia=%s", id)
	res, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}
	var data RCCURLJSON
	if err = json.NewDecoder(res.Body).Decode(&data); err != nil {
		fmt.Println(url)
		return "", err
	}
	return data.URL, nil
}

func listRCCEpisodesFromURL(url string) ([]string, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	links := []string{}
	var link string
	// Find the review items
	doc.Find(".medianet-content").Each(func(i int, s *goquery.Selection) {
		// For each item found, get the band and title
		link, _ = s.Attr("href")
		if len(link) > 0 {
			links = append(links, link)
		}
	})
	return links, nil
}

func toutTv(showName string) {
	resp, err := query(presQuery(showName))
	if err != nil {
		log.Printf("Something went wrong connecting to the server: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("The server didn't respond with the expected status code, got: %d\n", resp.StatusCode)
		body, _ := ioutil.ReadAll(resp.Body)
		log.Println(string(body))
		os.Exit(1)
	}
	var data PresentationResponse
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Fatalf("Failed to parse the response data - %v", err)
	}
	for _, lineup := range data.SeasonLineups {
		if lineup.Name == "single" {
			for i, ep := range lineup.LineupItems {
				id := ep.Key[6:]
				fmt.Printf("%d - ID: %s - Title: %s (template: %s)\n", i,
					id, ep.Title, ep.Template)
				// pid := ep.IDMedia
				if ep.Template == "media" {

				}
			}
			break
		}
	}

}

func query(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; U; CPU iPhone OS 5_0 like Mac OS X; en-us) AppleWebKit/532.9 (KHTML, like Gecko) Version/5.0.5 Mobile/8A293 Safari/6531.22.7")
	req.Header.Set("Content-Type", "application/json")

	return http.DefaultClient.Do(req)
}

func presQuery(showKey string) string {
	return fmt.Sprintf("%s%s?v=2&d=android&excludeLineups=0", presentationURL, showKey)
}

type RCCURLJSON struct {
	URL       string      `json:"url"`
	Message   interface{} `json:"message"`
	ErrorCode int         `json:"errorCode"`
	Params    []struct {
		Name  string      `json:"name"`
		Value interface{} `json:"value"`
	} `json:"params"`
	Bitrates []struct {
		Bitrate int         `json:"bitrate"`
		Width   int         `json:"width"`
		Height  int         `json:"height"`
		Lines   string      `json:"lines"`
		Param   interface{} `json:"param"`
	} `json:"bitrates"`
}

// RCCEpisodeJSON is the JSON structure for the show information available in
// the episode's page.
type RCCEpisodeJSON struct {
	AppCode string `json:"appCode"`
	IDMedia string `json:"idMedia"`
	Params  struct {
		AutoPlay       bool   `json:"autoPlay"`
		CanExtract     bool   `json:"canExtract"`
		CanFullScreen  bool   `json:"canFullScreen"`
		Gui            string `json:"gui"`
		Height         string `json:"height"`
		ID             string `json:"id"`
		InfoBar        bool   `json:"infoBar"`
		IsNextable     bool   `json:"isNextable"`
		IsPreviousable bool   `json:"isPreviousable"`
		Lang           string `json:"lang"`
		Next           bool   `json:"next"`
		Previous       bool   `json:"previous"`
		Share          bool   `json:"share"`
		Time           string `json:"time"`
		URLTeaser      string `json:"urlTeaser"`
		Width          string `json:"width"`
	} `json:"params"`
}

type PresentationResponse struct {
	ExternalLinks []struct {
		Title    string `json:"Title"`
		Text     string `json:"Text"`
		URL      string `json:"Url"`
		ImageURL string `json:"ImageUrl"`
		LogoName string `json:"LogoName"`
		LogoURL  string `json:"LogoUrl"`
	} `json:"ExternalLinks"`
	ITunesLink               interface{} `json:"ITunesLink"`
	CreditStartTimeInSeconds float64     `json:"CreditStartTimeInSeconds"`
	LengthInSeconds          float64     `json:"LengthInSeconds"`
	OtherLineups             []struct {
		SelectedIndex interface{} `json:"SelectedIndex"`
		Name          string      `json:"Name"`
		Title         string      `json:"Title"`
		HasURL        bool        `json:"HasUrl"`
		URL           interface{} `json:"Url"`
		Ratio         string      `json:"Ratio"`
		Color         string      `json:"Color"`
		LineupItems   []struct {
			BookmarkKey      string      `json:"BookmarkKey"`
			Key              string      `json:"Key"`
			AppleKey         string      `json:"AppleKey"`
			Template         string      `json:"Template"`
			Title            string      `json:"Title"`
			IsFree           bool        `json:"IsFree"`
			IsDrm            bool        `json:"IsDrm"`
			IsActive         bool        `json:"IsActive"`
			Description      string      `json:"Description"`
			PromoDescription interface{} `json:"PromoDescription"`
			ImageURL         string      `json:"ImageUrl"`
			URL              string      `json:"Url"`
			TrackingURL      interface{} `json:"TrackingUrl"`
			Details          struct {
				Rating    interface{} `json:"Rating"`
				Networks  interface{} `json:"Networks"`
				Country   interface{} `json:"Country"`
				AirDate   interface{} `json:"AirDate"`
				Copyright interface{} `json:"Copyright"`
				Persons   interface{} `json:"Persons"`
				Tags      []struct {
					Key   string `json:"Key"`
					Value []struct {
						ID                   int           `json:"Id"`
						URL                  string        `json:"Url"`
						Title                string        `json:"Title"`
						TypeTag              string        `json:"TypeTag"`
						UniversalSearchGenre string        `json:"UniversalSearchGenre"`
						ChildTags            []interface{} `json:"ChildTags"`
						ParentTag            interface{}   `json:"ParentTag"`
					} `json:"Value"`
				} `json:"Tags"`
				Length         int         `json:"Length"`
				Description    string      `json:"Description"`
				DetailsTitle   string      `json:"DetailsTitle"`
				ImageURL       string      `json:"ImageUrl"`
				ProductionYear int         `json:"ProductionYear"`
				LengthText     interface{} `json:"LengthText"`
				OriginalTitle  interface{} `json:"OriginalTitle"`
				Type           string      `json:"Type"`
			} `json:"Details"`
			Details2             interface{} `json:"Details2"`
			DepartureDescription interface{} `json:"DepartureDescription"`
			MigrationDescription interface{} `json:"MigrationDescription"`
			ArrivalDescription   interface{} `json:"ArrivalDescription"`
			Share                struct {
				ShareTitle  string `json:"ShareTitle"`
				URL         string `json:"Url"`
				AbsoluteURL string `json:"AbsoluteUrl"`
			} `json:"Share"`
			Length           interface{} `json:"Length"`
			FilterValueA     string      `json:"FilterValueA"`
			IsGeolocalized   bool        `json:"IsGeolocalized"`
			HasNewEpisodes   bool        `json:"HasNewEpisodes"`
			ExcludeDevice    interface{} `json:"ExcludeDevice"`
			LogoTargettingID interface{} `json:"LogoTargettingId"`
		} `json:"LineupItems"`
		HasLineupNavigation     bool          `json:"HasLineupNavigation"`
		IsFree                  bool          `json:"IsFree"`
		LineupNavigationItems   interface{}   `json:"LineupNavigationItems"`
		Header                  interface{}   `json:"Header"`
		FilterValueA            interface{}   `json:"FilterValueA"`
		FilterValueB            interface{}   `json:"FilterValueB"`
		LineupItemFiltersA      []interface{} `json:"LineupItemFiltersA"`
		ActiveLineupItemFilterA interface{}   `json:"ActiveLineupItemFilterA"`
		Behaviour               string        `json:"Behaviour"`
		LineupItemTextTemplate  string        `json:"LineupItemTextTemplate"`
		Theme                   interface{}   `json:"Theme"`
	} `json:"OtherLineups"`
	SelectedSeasonName string `json:"SelectedSeasonName"`
	SeasonLineups      []struct {
		SelectedIndex int         `json:"SelectedIndex"`
		Name          string      `json:"Name"`
		Title         string      `json:"Title"`
		HasURL        bool        `json:"HasUrl"`
		URL           interface{} `json:"Url"`
		Ratio         string      `json:"Ratio"`
		Color         string      `json:"Color"`
		LineupItems   []struct {
			IDMedia          string      `json:"IdMedia"`
			AppCode          string      `json:"AppCode"`
			CapsuleType      interface{} `json:"CapsuleType"`
			IsNew            bool        `json:"IsNew"`
			NoFMC            interface{} `json:"NoFMC"`
			IsAvailable      bool        `json:"IsAvailable"`
			BookmarkKey      string      `json:"BookmarkKey"`
			Key              string      `json:"Key"`
			AppleKey         string      `json:"AppleKey"`
			Template         string      `json:"Template"`
			Title            string      `json:"Title"`
			IsFree           bool        `json:"IsFree"`
			IsDrm            bool        `json:"IsDrm"`
			IsActive         bool        `json:"IsActive"`
			Description      string      `json:"Description"`
			PromoDescription interface{} `json:"PromoDescription"`
			ImageURL         string      `json:"ImageUrl"`
			URL              string      `json:"Url"`
			TrackingURL      interface{} `json:"TrackingUrl"`
			Details          struct {
				Rating    string        `json:"Rating"`
				Networks  interface{}   `json:"Networks"`
				Country   interface{}   `json:"Country"`
				AirDate   string        `json:"AirDate"`
				Copyright string        `json:"Copyright"`
				Persons   []interface{} `json:"Persons"`
				Tags      []struct {
					Key   string `json:"Key"`
					Value []struct {
						ID                   int           `json:"Id"`
						URL                  string        `json:"Url"`
						Title                string        `json:"Title"`
						TypeTag              string        `json:"TypeTag"`
						UniversalSearchGenre string        `json:"UniversalSearchGenre"`
						ChildTags            []interface{} `json:"ChildTags"`
						ParentTag            interface{}   `json:"ParentTag"`
					} `json:"Value"`
				} `json:"Tags"`
				Length         int    `json:"Length"`
				Description    string `json:"Description"`
				DetailsTitle   string `json:"DetailsTitle"`
				ImageURL       string `json:"ImageUrl"`
				ProductionYear int    `json:"ProductionYear"`
				LengthText     string `json:"LengthText"`
				OriginalTitle  string `json:"OriginalTitle"`
				Type           string `json:"Type"`
			} `json:"Details"`
			Details2             interface{} `json:"Details2"`
			DepartureDescription interface{} `json:"DepartureDescription"`
			MigrationDescription interface{} `json:"MigrationDescription"`
			ArrivalDescription   interface{} `json:"ArrivalDescription"`
			Share                struct {
				ShareTitle  string `json:"ShareTitle"`
				URL         string `json:"Url"`
				AbsoluteURL string `json:"AbsoluteUrl"`
			} `json:"Share"`
			Length           interface{} `json:"Length"`
			FilterValueA     interface{} `json:"FilterValueA"`
			IsGeolocalized   bool        `json:"IsGeolocalized"`
			HasNewEpisodes   bool        `json:"HasNewEpisodes"`
			ExcludeDevice    string      `json:"ExcludeDevice"`
			LogoTargettingID interface{} `json:"LogoTargettingId"`
		} `json:"LineupItems"`
		HasLineupNavigation     bool          `json:"HasLineupNavigation"`
		IsFree                  bool          `json:"IsFree"`
		LineupNavigationItems   interface{}   `json:"LineupNavigationItems"`
		Header                  interface{}   `json:"Header"`
		FilterValueA            interface{}   `json:"FilterValueA"`
		FilterValueB            interface{}   `json:"FilterValueB"`
		LineupItemFiltersA      []interface{} `json:"LineupItemFiltersA"`
		ActiveLineupItemFilterA interface{}   `json:"ActiveLineupItemFilterA"`
		Behaviour               string        `json:"Behaviour"`
		LineupItemTextTemplate  interface{}   `json:"LineupItemTextTemplate"`
		Theme                   interface{}   `json:"Theme"`
	} `json:"SeasonLineups"`
	SeasonLineupsTitle interface{} `json:"SeasonLineupsTitle"`
	HasPlayButton      bool        `json:"HasPlayButton"`
	PlayButtonText     string      `json:"PlayButtonText"`
	PlayButtonText2    string      `json:"PlayButtonText2"`
	StatsMetas         struct {
		Description                     string `json:"description"`
		RcDomaine                       string `json:"rc.domaine"`
		RcApplication                   string `json:"rc.application"`
		RcFormatapplication             string `json:"rc.formatapplication"`
		RcSection                       string `json:"rc.section"`
		RcGroupesection                 string `json:"rc.groupesection"`
		RcListe                         string `json:"rc.liste"`
		RcSegment                       string `json:"rc.segment"`
		RcPagesegment                   string `json:"rc.pagesegment"`
		RcCodepage                      string `json:"rc.codepage"`
		RcNiveau                        string `json:"rc.niveau"`
		RcEmission                      string `json:"rc.emission"`
		RcCodeemission                  string `json:"rc.codeemission"`
		RcTitre                         string `json:"rc.titre"`
		RcSaison                        string `json:"rc.saison"`
		RcEpisode                       string `json:"rc.episode"`
		RcCollection                    string `json:"rc.collection"`
		RcGenre                         string `json:"rc.genre"`
		RcVientdemaliste                string `json:"rc.vientdemaliste"`
		RcAcces                         string `json:"rc.acces"`
		RcTelco                         string `json:"rc.telco"`
		RcPlan                          string `json:"rc.plan"`
		RcForfait                       string `json:"rc.forfait"`
		RcIdcampagne                    string `json:"rc.idcampagne"`
		AppleMediaServiceSubscriptionV2 string `json:"apple-media-service-subscription-v2"`
		Keywords                        string `json:"keywords"`
		RcExtra                         string `json:"rc.extra"`
		FbAppID                         string `json:"fb:app_id"`
		OgTitle                         string `json:"og:title"`
		OgDescription                   string `json:"og:description"`
		OgURL                           string `json:"og:url"`
		OgType                          string `json:"og:type"`
		OgImage                         string `json:"og:image"`
	} `json:"StatsMetas"`
	MediaURL                   string      `json:"MediaUrl"`
	MediaTitle                 string      `json:"MediaTitle"`
	BackgroundImageURL         interface{} `json:"BackgroundImageUrl"`
	IsPubMandatoryForAllUsers  bool        `json:"IsPubMandatoryForAllUsers"`
	IsPubMandatoryForFreeUsers bool        `json:"IsPubMandatoryForFreeUsers"`
	ShowPub                    bool        `json:"ShowPub"`
	IDMedia                    string      `json:"IdMedia"`
	AppCode                    string      `json:"AppCode"`
	CapsuleType                interface{} `json:"CapsuleType"`
	IsNew                      bool        `json:"IsNew"`
	NoFMC                      interface{} `json:"NoFMC"`
	IsAvailable                bool        `json:"IsAvailable"`
	BookmarkKey                string      `json:"BookmarkKey"`
	Key                        string      `json:"Key"`
	AppleKey                   string      `json:"AppleKey"`
	Template                   string      `json:"Template"`
	Title                      string      `json:"Title"`
	IsFree                     bool        `json:"IsFree"`
	IsDrm                      bool        `json:"IsDrm"`
	IsActive                   bool        `json:"IsActive"`
	Description                string      `json:"Description"`
	PromoDescription           interface{} `json:"PromoDescription"`
	ImageURL                   string      `json:"ImageUrl"`
	URL                        interface{} `json:"Url"`
	TrackingURL                interface{} `json:"TrackingUrl"`
	Details                    struct {
		Rating    interface{} `json:"Rating"`
		Networks  interface{} `json:"Networks"`
		Country   interface{} `json:"Country"`
		AirDate   interface{} `json:"AirDate"`
		Copyright interface{} `json:"Copyright"`
		Persons   interface{} `json:"Persons"`
		Tags      []struct {
			Key   string `json:"Key"`
			Value []struct {
				ID                   int           `json:"Id"`
				URL                  string        `json:"Url"`
				Title                string        `json:"Title"`
				TypeTag              string        `json:"TypeTag"`
				UniversalSearchGenre string        `json:"UniversalSearchGenre"`
				ChildTags            []interface{} `json:"ChildTags"`
				ParentTag            interface{}   `json:"ParentTag"`
			} `json:"Value"`
		} `json:"Tags"`
		Length         int         `json:"Length"`
		Description    string      `json:"Description"`
		DetailsTitle   string      `json:"DetailsTitle"`
		ImageURL       string      `json:"ImageUrl"`
		ProductionYear int         `json:"ProductionYear"`
		LengthText     interface{} `json:"LengthText"`
		OriginalTitle  interface{} `json:"OriginalTitle"`
		Type           string      `json:"Type"`
	} `json:"Details"`
	Details2             interface{} `json:"Details2"`
	DepartureDescription interface{} `json:"DepartureDescription"`
	MigrationDescription interface{} `json:"MigrationDescription"`
	ArrivalDescription   interface{} `json:"ArrivalDescription"`
	Share                struct {
		ShareTitle  string `json:"ShareTitle"`
		URL         string `json:"Url"`
		AbsoluteURL string `json:"AbsoluteUrl"`
	} `json:"Share"`
	Length           interface{} `json:"Length"`
	FilterValueA     string      `json:"FilterValueA"`
	IsGeolocalized   bool        `json:"IsGeolocalized"`
	HasNewEpisodes   bool        `json:"HasNewEpisodes"`
	ExcludeDevice    interface{} `json:"ExcludeDevice"`
	LogoTargettingID interface{} `json:"LogoTargettingId"`
}
