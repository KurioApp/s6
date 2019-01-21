package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/KurioApp/s6"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfg string

	cmd = &cobra.Command{
		Use: "s6-agent",
		Run: runServer,
	}

	errForbidden = errors.New("got 503 status")
	maxHTTPTries = 5
)

func main() {
	cobra.OnInitialize(initConfig)
	cmd.PersistentFlags().StringVar(&cfg, "config", "./config.json", "JSON file consists all configurations")

	if err := cmd.Execute(); err != nil {
		logrus.Fatalf("Error running agent: %v", err)
	}
}

func initConfig() {
	viper.SetConfigType("json")
	viper.AddConfigPath(".")

	viper.SetDefault("http_address", ":80")
	viper.SetDefault("base_dir", "/tmp")

	if cfg != "" {
		viper.SetConfigFile(cfg)
	}

	if err := viper.ReadInConfig(); err != nil {
		logrus.Fatalf("Failed loading configs: %s", cfg)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	e := echo.New()

	e.GET("ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "pong")
	})

	e.POST("sync", func(c echo.Context) error {
		var fileObj s6.S3File

		if err := c.Bind(&fileObj); err != nil {
			return c.NoContent(http.StatusUnprocessableEntity)
		}

		fileType := bucketToType(fileObj.Bucket)

		switch fileType {
		case "video":
			go processVideo(fileObj)

		case "image":
			go processImage(fileObj)

		default:
			return c.String(http.StatusInternalServerError, "Unknown bucket")
		}

		return c.NoContent(http.StatusOK)
	})

	httpAddress := viper.GetString("http_address")
	e.Start(httpAddress)
}

func processImage(fileObj s6.S3File) {
	log := logrus.WithField("file", fileObj)

	paths := viper.GetStringSlice("thumbor.paths")
	for _, path := range paths {
		err := hitThumborCache(fileObj, path)
		if err != nil {
			log.Error(err)
		}
	}
}

func processVideo(fileObj s6.S3File) {
	log := logrus.WithField("file", fileObj)

	for i := 0; i < maxHTTPTries; i++ {
		log = log.WithField("tries", i+1)
		log.Info("Start downloading file")

		err := download(fileObj)
		if err == errForbidden {
			log.Info("Got forbidden")
			time.Sleep(1 * time.Second)
			continue

		}

		if err != nil {
			log.Error(err)
		} else {
			log.Info("Done")
		}

		return
	}

	log.Error("Max tries exceeded")
}

func download(fileObj s6.S3File) error {
	baseDir := viper.GetString("base_dir")
	fileDir := filepath.Dir(fileObj.Key)

	if err := os.MkdirAll(filepath.Join(baseDir, fileDir), os.ModePerm); err != nil {
		return fmt.Errorf("failed creating dir: %v", err)
	}

	tempName := fmt.Sprintf("temp-file-%s", strconv.Itoa(rand.Intn(9999999)))
	tempFile := filepath.Join(baseDir, tempName)

	f, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed creating file: %v", err)
	}

	resp, err := http.Get(fileObj.URL())
	if err != nil || resp == nil {
		return fmt.Errorf("error downloading file: %v", err)
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode == http.StatusForbidden {
		return errForbidden
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("got non-OK when downloading file: %v", resp.StatusCode)
	}

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("failed storing file: %v", err)
	}

	err = os.Rename(tempFile, filepath.Join(baseDir, fileObj.Key))
	if err != nil {
		return fmt.Errorf("failed renaming file: %v", err)
	}

	return nil
}

func hitThumborCache(fileObj s6.S3File, thumborOpt string) error {
	log := logrus.WithField("file", fileObj)

	thumborURL := viper.GetString("thumbor.url")
	if thumborURL == "" {
		return errors.New("thumbor URL not set")
	}

	originalURL := fmt.Sprintf("https://%s/%s", fileObj.Bucket, fileObj.Key)
	thumborPath := thumborOpt + "/" + originalURL
	log = log.WithField("thumbor path", thumborPath)

	thumborKey := viper.GetString("thumbor.key")

	key := "unsafe"
	if thumborKey != "" {
		hash := hmac.New(sha1.New, []byte(thumborKey))
		hash.Write([]byte(thumborPath))
		message := hash.Sum(nil)
		key = base64.URLEncoding.EncodeToString(message)
	}

	imgURL := fmt.Sprintf("%s/%s/%s", thumborURL, key, thumborPath)

	for i := 0; i < maxHTTPTries; i++ {
		log = log.WithField("tries", i+1)

		resp, err := http.Get(imgURL)
		if err != nil {
			return err
		}

		if resp == nil {
			return errors.New("got nil response")
		}

		cacheHeader := strings.ToLower(resp.Header.Get("X-Cache"))
		if !strings.HasPrefix(cacheHeader, "hit") {
			log.WithField("headers", resp.Header).Info("Cache not hit")
			continue
		} else {
			log.Info("Cache hit")
			return nil
		}
	}

	return errors.New("max tries exceeded")
}

func bucketToType(bucket string) string {
	imgBuckets := viper.GetStringSlice("bucket.image")
	for _, b := range imgBuckets {
		if bucket == b {
			return "image"
		}
	}

	videoBuckets := viper.GetStringSlice("bucket.video")
	for _, b := range videoBuckets {
		if bucket == b {
			return "video"
		}
	}

	return ""
}
