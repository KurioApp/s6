package main

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

		go func() {
			maxTries := 5
			log := logrus.WithField("file", fileObj)

			for i := 0; i < maxTries; i++ {
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
		}()

		return c.NoContent(http.StatusOK)
	})

	httpAddress := viper.GetString("http_address")
	e.Start(httpAddress)
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
