package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var version = "local"

var app struct {
  Status bool `json:"status"`
  ISP string `json:"isp"`
  IpAddress string `json:"ipAddress"`
  LastCheck time.Time `json:"lastChek"`
  Proxy string
}
// var version = "http://192.168.0.156:4001"

func main() {
  flag.StringVar(&app.Proxy, "proxy", "", "HTTP proxy URL to use")
  flag.StringVar(&app.ISP, "isp", "-", "ISP name for identification")
  flag.Parse()

  fmt.Println("checking with the following proxy:", app.Proxy)

  proxyUrl, err := url.Parse(app.ISP)
  if err != nil {
    fmt.Println("Error parsing proxy URL:", err)
    return
  }

  client := &http.Client{
    Timeout: time.Second * 10,
  }

  if app.Proxy != "" {
    client.Transport = &http.Transport{
      Proxy: http.ProxyURL(proxyUrl),
    }
  }


  ticker := time.Tick(time.Minute)
  sigChan := make(chan os.Signal, 1)

  signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

  go func() {
    for {
      select {
      case <- ticker:
        fmt.Println("Getting public IP")
        publicIp, err := getPublicIp(client)
        if err != nil {
          fmt.Println("Error getting public IP:", err)
          app.Status = false
          return
        }

        app.Status = true
        app.IpAddress = publicIp
        app.LastCheck = time.Now()
      case <- sigChan:
        return
      }
    }
  }()

  http.HandleFunc("/", func( res http.ResponseWriter, req *http.Request) {
    var response struct {
      ErrorCode int32 `json:"errorCode"`
      Message string `json:"message"`
      Data interface{} `json:"data"`
    }

    response.ErrorCode = 0
    response.Message = "success"
    response.Data = app

    resBody, err := json.Marshal(response)
    if err != nil {
      fmt.Println("Error marshalling response body:", err)
      http.Error(res, err.Error(), http.StatusInternalServerError)
      return
    }

    res.Header().Add("Content-Type", "application/json")
    res.Write(resBody)
  })

  http.HandleFunc("/check", func( res http.ResponseWriter, req *http.Request) {
    if req.Method == "POST" {
      var checkRequestBody struct {
        Domain string `json:"domain"`
      }

      body, err := io.ReadAll(req.Body)
      if err != nil {
        http.Error(res, err.Error(), http.StatusBadRequest)
        return
      }

      defer req.Body.Close()

      if err := json.Unmarshal(body, &checkRequestBody); err != nil {
        http.Error(res, err.Error(), http.StatusBadRequest)
        return
      }

      fmt.Println("Visiting", checkRequestBody.Domain)

      var response struct {
        ErrorCode int32 `json:"errorCode"`
        Message string `json:"message"`
        Data interface{} `json:"data"`
      }

      vres, err := visitDomain(client, checkRequestBody.Domain)
      if err != nil {
        response.ErrorCode = 500
        response.Message = fmt.Sprintf("domain unreachable: %s", err.Error())

        resBody, err := json.Marshal(response)
        if err != nil {
          fmt.Println("Error marshalling response body:", err)
        }

        res.Header().Add("Content-Type", "application/json")
        res.Write(resBody)
        return
      }

      response.ErrorCode = 0
      response.Message = "success"
      response.Data = vres

      resBody, err := json.Marshal(response)
      if err != nil {
        fmt.Println("Error marshalling response body:", err)
      }

      res.Header().Add("Content-Type", "application/json")
      res.Write(resBody)
    }
  })

  go func() {
    fmt.Println("Listening on port 8082")
    http.ListenAndServe(":8082", nil)
  }()

  s := <-sigChan

  fmt.Println("Application stopped:", s)
}

func getPublicIp(client *http.Client) (string, error) {
  res, err := client.Get("https://httpbin.org/ip")
  if err != nil {
    fmt.Println("Error creating HTTP request:", err)
    return "", err
  }

  defer res.Body.Close()

  body, err := io.ReadAll(res.Body)
  if err != nil {
    fmt.Println("Error reading response body:", err)
    return "", err
  }

  var out struct {
    Origin string `json:"origin"`
  }

  if err := json.Unmarshal(body, &out); err != nil {
    fmt.Println("Error unmarshalling response body:", err)
    fmt.Println("Response Body:", string(body))
    return "", err
  }

  return out.Origin, nil
}

// This function will return the latest redirect URL
func visitDomain(client *http.Client, domain string) (string, error) {
  res, err := client.Get(domain)
  if err != nil {
    fmt.Println("Error creating HTTP request:", err)
    return "", err
  }

  return res.Request.URL.String(), nil
}
