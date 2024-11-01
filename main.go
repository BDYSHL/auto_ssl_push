/*
 * Copyright 2024 BDYSHL
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"autoUCDNcert/config"
	"autoUCDNcert/ucloud"
	"autoUCDNcert/utils"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func main() {
	configUrl := ""
	env := flag.String("env", "", "env fro environment")
	if *env == "dev" {
		configUrl = "base_config.yml"
	} else {
		configUrl = "config.yml"
	}
	// 获取当前工作目录
	workDir, err := os.Getwd()
	if err != nil {
		fmt.Println("获取工作目录失败: ", err)
		return
	}
	// 构建 config 文件的完整路径
	configPath := filepath.Join(workDir, configUrl)
	config.InitConfig(configPath)

	var mu sync.Mutex

	domainID := flag.String("domainID", "", "Domain ID for UCloud CDN")
	certName := flag.String("certName", "", "Certificate Name")
	certPath := flag.String("certPath", "", "Path to the certificate file")
	keyPath := flag.String("keyPath", "", "Path to the private key file")
	flag.Parse()

	if *certName == "" || *certPath == "" || *keyPath == "" {
		fmt.Println("All parameters (certName, certPath, keyPath) are required.")
		return
	}

	userCert, err := utils.ReadFile(*certPath)
	if err != nil {
		fmt.Println("读取证书文件失败: ", err)
		return
	}

	userKey, err := utils.ReadFile(*keyPath)
	if err != nil {
		fmt.Println("读取密钥文件失败: ", err)
		return
	}

	logFile, err := os.OpenFile("operation_log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("无法创建日志文件: ", err)
		return
	}
	defer logFile.Close()

	client := ucloud.NewClient(logFile)

	var domainIDs []string
	if *domainID == "" {
		domainIDs, err = client.GetDomainIDsForCert(*certName)
		if err != nil {
			client.LogOperation(fmt.Sprintf("获取域名 ID 失败: %v", err))
		} else if len(domainIDs) == 0 {
			client.LogOperation("未找到任何域名 ID, 继续执行后续操作")
		}
	} else {
		domainIDs = append(domainIDs, *domainID)
	}

	client.LogOperation("===================START==========================")
	for _, domainID := range domainIDs {
		mu.Lock()
		err := client.ChangeHttps(domainID, "disable", "")
		mu.Unlock()
		if err != nil {
			client.LogOperation("关闭 HTTPS 失败，终止操作: " + err.Error())
			return
		}
		time.Sleep(2 * time.Second)
	}

	mu.Lock()
	err = client.DeleteCert(*certName)
	mu.Unlock()
	if err != nil {
		client.LogOperation("删除证书失败，终止操作: " + err.Error())
		return
	}
	time.Sleep(2 * time.Second)

	mu.Lock()
	err = client.UploadCert(*certName, userCert, userKey)
	mu.Unlock()
	if err != nil {
		client.LogOperation("上传证书失败，终止操作: " + err.Error())
		return
	}
	time.Sleep(30 * time.Second)

	for _, domainID := range domainIDs {
		mu.Lock()
		err := client.ChangeHttps(domainID, "enable", *certName)
		mu.Unlock()
		if err != nil {
			client.LogOperation(fmt.Sprintf("开启 HTTPS 失败, DomainID: %s, 错误: %v", domainID, err))
			client.HandleFailure("ChangeHttps", err)
		}
		time.Sleep(5 * time.Second)
	}
	client.LogOperation("===================END==========================")
	client.LogOperation("")
	client.LogOperation("")
}
