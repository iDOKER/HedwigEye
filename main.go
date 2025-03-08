package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
)

type Config struct {
	Server struct {
		Port int
	}
	Database struct {
		Host     string
		Port     int
		User     string
		Password string
		Name     string
	}
	Alert struct {
		DefaultDays int
	}
}

var config Config
var db *sql.DB

func LoadConfig() {
	log.Println("加载配置...")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("配置解析失败: %v", err)
	}
	log.Println("配置加载成功")
}

func InitDB() {
	log.Println("初始化数据库连接...")
	var err error
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		config.Database.User, config.Database.Password,
		config.Database.Host, config.Database.Port, config.Database.Name)
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	log.Println("数据库连接成功")
}

func GetAlerts(w http.ResponseWriter, r *http.Request) {
    // 添加CORS头
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

    log.Println("收到 API 请求: /api/alerts")
    startDate := r.URL.Query().Get("start")
    endDate := r.URL.Query().Get("end")
    if startDate == "" || endDate == "" {
        endDate = time.Now().Format("2006-01-02")
        startDate = time.Now().AddDate(0, 0, -config.Alert.DefaultDays).Format("2006-01-02")
    }
    log.Printf("查询告警数据: 开始日期=%s, 结束日期=%s", startDate, endDate)

    // 将日期字符串转换为时间对象
    startTime, err := time.Parse("2006-01-02", startDate)
    if err != nil {
        log.Printf("日期解析失败: %v", err)
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    endTime, err := time.Parse("2006-01-02", endDate)
    if err != nil {
        log.Printf("日期解析失败: %v", err)
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // 将时间对象转换为 UNIX 时间戳
    startTimestamp := startTime.Unix()
    endTimestamp := endTime.Unix()

    query := `SELECT first_trigger_time, rule_name, group_name, COUNT(*) as count 
			FROM alert_his_event 
			WHERE first_trigger_time BETWEEN ? AND ?
			GROUP BY first_trigger_time, rule_name, group_name;`

    rows, err := db.Query(query, startTimestamp, endTimestamp)
    if err != nil {
        log.Printf("数据库查询失败: %v", err)
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    data := make(map[string]map[string]int)
    groupData := make(map[string]map[string]int)

    for rows.Next() {
        var firstTriggerTime int64
        var ruleName, groupName string
        var count int
        if err := rows.Scan(&firstTriggerTime, &ruleName, &groupName, &count); err != nil {
            log.Printf("数据扫描失败: %v", err)
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        // 将 UNIX 时间戳转换为日期字符串
        date := time.Unix(firstTriggerTime, 0).Format("2006-01-02")
        if data[date] == nil {
            data[date] = make(map[string]int)
        }
        if groupData[date] == nil {
            groupData[date] = make(map[string]int)
        }
        data[date][ruleName] += count
        groupData[date][groupName] += count
    }

    result := []map[string]interface{}{}
    for date, rules := range data {
        entry := map[string]interface{}{
            "date":              date,
            "rule_name_counts":  []map[string]interface{}{},
            "group_name_counts": []map[string]interface{}{},
        }
        for name, count := range rules {
            entry["rule_name_counts"] = append(entry["rule_name_counts"].([]map[string]interface{}), map[string]interface{}{"name": name, "count": count})
        }
        for name, count := range groupData[date] {
            entry["group_name_counts"] = append(entry["group_name_counts"].([]map[string]interface{}), map[string]interface{}{"name": name, "count": count})
        }
        result = append(result, entry)
    }

    log.Printf("返回数据: %d 条记录", len(result))
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{"data": result})
}

func main() {
	log.Println("服务启动...")
	LoadConfig()
	InitDB()
	http.HandleFunc("/api/alerts", GetAlerts)
	port := fmt.Sprintf(":%d", config.Server.Port)
	log.Printf("服务器运行在 http://localhost%s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
