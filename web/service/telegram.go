package service

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"time"
	"x-ui/logger"
	"x-ui/util/common"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
)

// This should be global variable, and only one instance
var botInstace *tgbotapi.BotAPI

// 结构体类型大写表示可以被其他包访问
type TelegramService struct {
	xrayService    XrayService
	serverService  ServerService
	inboundService InboundService
	settingService SettingService
}

func (s *TelegramService) GetsystemStatus() string {
	var status string
	// get hostname
	name, err := os.Hostname()
	if err != nil {
		fmt.Println("get hostname error: ", err)
		return ""
	}

	status = fmt.Sprintf("主机名称: %s\r\n", name)
	status += fmt.Sprintf("系统类型: %s\r\n", runtime.GOOS)
	status += fmt.Sprintf("CPU架构: %s\r\n", runtime.GOARCH)

	avgState, err := load.Avg()
	if err != nil {
		logger.Warning("get load avg failed: ", err)
	} else {
		status += fmt.Sprintf("系统负载: %.2f, %.2f, %.2f\r\n", avgState.Load1, avgState.Load5, avgState.Load15)
	}

	upTime, err := host.Uptime()
	if err != nil {
		logger.Warning("get uptime failed: ", err)
	} else {
		status += fmt.Sprintf("运行时间: %s\r\n", common.FormatTime(upTime))
	}

	// xray version
	status += fmt.Sprintf("目前xray内核版本: %s\r\n", s.xrayService.GetXrayVersion())

	// ip address
	var ip string
	ip = common.GetMyIpAddr()
	status += fmt.Sprintf("IP地址: %s\r\n \r\n", ip)

	// get traffic
	inbouds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		logger.Warning("StatsNotifyJob run error: ", err)
	}

	for _, inbound := range inbouds {
		status += fmt.Sprintf("节点名称: %s\r\n端口: %d\r\n上行流量↑: %s\r\n下行流量↓: %s\r\n总流量: %s\r\n", inbound.Remark, inbound.Port, common.FormatTraffic(inbound.Up), common.FormatTraffic(inbound.Down), common.FormatTraffic((inbound.Up + inbound.Down)))
		if inbound.ExpiryTime == 0 {
			status += fmt.Sprintf("到期时间: 无限期\r\n \r\n")
		} else {
			status += fmt.Sprintf("到期时间: %s\r\n \r\n", time.Unix((inbound.ExpiryTime/1000), 0).Format("2006-01-02 15:04:05"))
		}
	}
	return status
}

func (s *TelegramService) StartRun() {
	logger.Info("telegram service ready to run")
	s.settingService = SettingService{}
	tgBottoken, err := s.settingService.GetTgBotToken()

	if err != nil || tgBottoken == "" {
		logger.Infof("Telegram service start run failed, GetTgBotToken fail, err: %v, tgBottoken: %s", err, tgBottoken)
		return
	}
	logger.Infof("TelegramService GetTgBotToken:%s", tgBottoken)

	botInstace, err = tgbotapi.NewBotAPI(tgBottoken)

	if err != nil {
		logger.Infof("Telegram service start run failed, NewBotAPI fail: %v, tgBottoken: %s", err, tgBottoken)
		return
	}
	botInstace.Debug = false
	fmt.Printf("Authorized on account %s", botInstace.Self.UserName)

	// get all my commands
	commands, err := botInstace.GetMyCommands()
	if err != nil {
		logger.Warning("Telegram service start run error, GetMyCommandsfail: ", err)
	}

	for _, command := range commands {
		fmt.Printf("Command %s, Description: %s \r\n", command.Command, command.Description)
	}

	// get update
	chanMessage := tgbotapi.NewUpdate(0)
	chanMessage.Timeout = 60

	updates := botInstace.GetUpdatesChan(chanMessage)

	for update := range updates {
		if update.Message == nil {
			// NOTE:may ther are different bot instance,we could use different bot endApiPoint
			updates.Clear()
			continue
		}

		if !update.Message.IsCommand() {
			continue
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

		// Extract the command from the Message.
		switch update.Message.Command() {
		case "delete":
			inboundPortStr := update.Message.CommandArguments()
			inboundPortValue, err := strconv.Atoi(inboundPortStr)

			if err != nil {
				msg.Text = "无效的入站端口，请检查"
				break
			}

			//logger.Infof("Will delete port:%d inbound", inboundPortValue)
			error := s.inboundService.DelInboundByPort(inboundPortValue)
			if error != nil {
				msg.Text = fmt.Sprintf("删除端口为 %d 的节点失败", inboundPortValue)
			} else {
				msg.Text = fmt.Sprintf("已成功删除端口为 %d 的节点", inboundPortValue)
			}

		case "restart":
			err := s.xrayService.RestartXray(true)
			if err != nil {
				msg.Text = fmt.Sprintln("重启xray服务失败, err: ", err)
			} else {
				msg.Text = "已成功重启xray服务"
			}

		case "disable":
			inboundPortStr := update.Message.CommandArguments()
			inboundPortValue, err := strconv.Atoi(inboundPortStr)
			if err != nil {
				msg.Text = "无效的入站端口，请检查"
				break
			}
			//logger.Infof("Will delete port:%d inbound", inboundPortValue)
			error := s.inboundService.DisableInboundByPort(inboundPortValue)
			if error != nil {
				msg.Text = fmt.Sprintf("禁用端口为 %d 的节点失败, err: %s", inboundPortValue, error)
			} else {
				msg.Text = fmt.Sprintf("已成功禁用端口为 %d 的节点", inboundPortValue)
			}

		case "enable":
			inboundPortStr := update.Message.CommandArguments()
			inboundPortValue, err := strconv.Atoi(inboundPortStr)
			if err != nil {
				msg.Text = "无效的入站端口，请检查"
				break
			}
			//logger.Infof("Will delete port:%d inbound", inboundPortValue)
			error := s.inboundService.EnableInboundByPort(inboundPortValue)
			if error != nil {
				msg.Text = fmt.Sprintf("尝试启用端口为 %d 的节点失败, err: %s", inboundPortValue, error)
			} else {
				msg.Text = fmt.Sprintf("已成功启用端口为 %d 的节点", inboundPortValue)
			}

		case "clear":
			inboundPortStr := update.Message.CommandArguments()
			inboundPortValue, err := strconv.Atoi(inboundPortStr)
			if err != nil {
				msg.Text = "无效的入站端口，请检查"
				break
			}
			error := s.inboundService.ClearTrafficByPort(inboundPortValue)
			if error != nil {
				msg.Text = fmt.Sprintf("清除端口为 %d 的节点流量失败, err: %s", inboundPortValue, error)
			} else {
				msg.Text = fmt.Sprintf("已成功清除端口为 %d 的节点流量", inboundPortValue)
			}

		case "clearall":
			error := s.inboundService.ClearAllInboundTraffic()
			if error != nil {
				msg.Text = fmt.Sprintf("清理所有节点流量失败, err: %s", error)
			} else {
				msg.Text = fmt.Sprintf("已成功清理所有节点流量")
			}

		case "version":
			versionStr := update.Message.CommandArguments()
			currentVersion, _ := s.serverService.GetXrayVersions()
			if currentVersion[0] == versionStr {
				msg.Text = fmt.Sprint("不能更新成和本地x-ui的xray内核一样的版本")
			}
			error := s.serverService.UpdateXray(versionStr)
			if error != nil {
				msg.Text = fmt.Sprintf("xray内核版本升级为 %s 失败, err: %s", versionStr, error)
			} else {
				msg.Text = fmt.Sprintf("xray内核版本升级为 %s 成功", versionStr)
			}

		case "status":
			msg.Text = s.GetsystemStatus()

		case "start":
			msg.Text = "欢迎使用x-ui面板机器人, 请输入 /help 查看帮助信息"

		default:
			// NOTE:here we need string as a new line each one,we should use ``
			msg.Text = `x-ui面版 Telegram Bot 使用说明
			
/help 获取bot的帮助信息 (此菜单)
/delete [PORT] 删除对应端口的节点
/restart 重启xray服务
/status 获取当前系统状态
/enable [PORT] 开启对应端口的节点
/disable [PORT] 关闭对应端口的节点
/clear [PORT] 清理对应端口的节点流量
/clearall 清理所有节点流量
/version [VERSION] 升级xray内核到 [VERSION] 版本
`
		}

		if _, err := botInstace.Send(msg); err != nil {
			log.Panic(err)
		}
	}

}

func (s *TelegramService) SendMsgToTgbot(msg string) {
	logger.Info("SendMsgToTgbot entered")
	tgBotid, err := s.settingService.GetTgBotChatId()
	if err != nil {
		logger.Warning("sendMsgToTgbot failed, GetTgBotChatId fail:", err)
		return
	}
	if tgBotid == 0 {
		logger.Warning("sendMsgToTgbot failed, GetTgBotChatId illegal")
		return
	}

	info := tgbotapi.NewMessage(int64(tgBotid), msg)
	if botInstace != nil {
		botInstace.Send(info)
	} else {
		logger.Warning("bot instance is nil")
	}
}

// NOTE:This function can't be called repeatly
func (s *TelegramService) StopRunAndClose() {
	if botInstace != nil {
		botInstace.StopReceivingUpdates()
	}
}
