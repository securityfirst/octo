package cmd

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/securityfirst/tent/utils"

	"github.com/securityfirst/tent/component"

	"github.com/securityfirst/tent/repo"
	"github.com/securityfirst/tent/transifex"
	"github.com/spf13/cobra"
)

// downloadCmd respresents the download command
var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Uploads latest contents to Transifex",
	Long:  `Downloads the lastest version of Transifex contents and uploads them to Tent.`,
	Run:   downloadRun,
}

func init() {
	transifexCmd.AddCommand(downloadCmd)
}

func downloadRun(cmd *cobra.Command, args []string) {
	r, err := repo.New(config.Github.Handler, config.Github.Project)
	if err != nil {
		log.Fatalf("Repo error: %s", err)
	}
	r.Pull()

	client := transifex.NewClient(config.Transifex.Project, config.Transifex.Username, config.Transifex.Password)
	client.RateLimit(time.Hour, 6000)

	parser := component.NewResourceParser()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)

	var count int

	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	go func() {
		defer func() { quit <- nil }()

		const lang = "zh-Hant"

		for _, cmp := range r.All("en") {
			count++
			var (
				resource     = cmp.Resource()
				translations = (map[string]string)(nil)
				cachePath    = filepath.Join(config.Root, resource.Slug)
			)
			if _, err := os.Stat(cachePath); os.IsNotExist(err) {
				translations, err = client.DownloadTranslations(resource.Slug)
				if err != nil {
					log.Printf("%s: %s", resource.Slug, red(err))
					continue
				}
				f, err := os.OpenFile(cachePath, os.O_WRONLY|os.O_CREATE, 0644)
				if err != nil {
					log.Printf("%s: %s", resource.Slug, red(err))
					continue
				}
				defer f.Close()
				if err := json.NewEncoder(f).Encode(translations); err != nil {
					log.Printf("%s: %s", resource.Slug, red(err))
					continue
				}
			} else if err == nil {
				f, err := os.Open(cachePath)
				if err != nil {
					log.Printf("%s: %s", resource.Slug, red(err))
					continue
				}
				if err = json.NewDecoder(f).Decode(&translations); err != nil {
					log.Printf("%s: %s", resource.Slug, red(err))
					continue
				}
			} else {
				log.Printf("%s: %s", resource.Slug, red(err))
				continue
			}

			target, ok := translations[lang]
			if !ok {
				log.Printf("%s: %s not found", resource.Slug, lang)
				continue
			}
			if err := ioutil.WriteFile(filepath.Join(config.Root, resource.Slug), []byte(target), 0666); err != nil {
				log.Printf("%s (%s) %s", resource.Slug, lang, err)
			}
			var m []map[string]string
			if err := json.NewDecoder(strings.NewReader(target)).Decode(&m); err != nil {
				log.Printf("%s (%s) %s\n%s", resource.Slug, lang, err, target)
				continue
			}
			if err := parser.Parse(cmp, &resource, lang[:2]); err != nil {
				log.Printf("%s (%s) %s", resource.Slug, lang, err)
				continue
			}
			if target != translations["en"] {
				log.Printf("%s %s - %s", green("translated"), lang, resource.Slug)
			} else {
				log.Printf("%s %s - %s", red("not translated"), lang, resource.Slug)
			}
		}
	}()

	<-quit

	var printCmp = func(cmp component.Component) {
		if err := utils.WriteCmp(config.Root, cmp); err != nil {
			log.Println(cmp.Path(), red(err))
			return
		}
		log.Println(cmp.Path(), green("ok"))
	}

	log.Printf("\n\n***** Saving %d files *****\n\n", count)
	for _, cats := range parser.Categories() {
		for _, cat := range cats {
			printCmp(cat)
			for _, s := range cat.Subcategories() {
				sub := cat.Sub(s)
				printCmp(sub)
				if check := sub.Checks(); check.HasChildren() {
					printCmp(check)
				}
				for _, i := range sub.ItemNames() {
					printCmp(sub.Item(i))
				}
			}
		}
	}

}
