package com

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

type UploadResponse struct {
	Error int
	Msg   string
	Data  Data
}

type Data struct {
	AppUrl      string  `json:"app_url"`
	OriFilename string  `json:"ori_filename"`
	UpFilename  string  `json:"up_filename"`
	AppHash     string  `json:"app_hash"`
	ImgW        string  `json:"imgW"`
	ImgH        string  `json:"imgH"`
	FileSize    float64 `json:"fileSize"`
	ImgType     string  `json:"imgType"`
}

//上传s3
func UploadFileS3(url string, params map[string]string, nameField, fileName string, localFile string) (*UploadResponse, error) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	formFile, err := writer.CreateFormFile(nameField, fileName)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(localFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	_, err = io.Copy(formFile, file)
	if err != nil {
		return nil, err
	}

	for key, val := range params {
		_ = writer.WriteField(key, val)
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	//req.Header.Set("Content-Type","multipart/form-data")
	req.Header.Add("Content-Type", writer.FormDataContentType())
	client := &http.Client{
		Timeout: 120 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ret := &UploadResponse{}
	err = json.Unmarshal(content, ret)
	if err != nil {
		return nil, err
	}
	return ret, nil
}
