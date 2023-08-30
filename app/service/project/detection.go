package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"path/filepath"
	"strconv"
	"sync"
	"yema.dev/app/model"
	"yema.dev/app/pkg/repo"
	"yema.dev/app/pkg/ssh"
	"yema.dev/app/service/common"
)

func sendDetectionMsgFn(ch chan<- *DetectionMsg) func(title, todo, err string, serverId int64) {
	return func(title, todo, err string, serverId int64) {
		ch <- &DetectionMsg{
			ServerId: serverId,
			Title:    title,
			Error:    err,
			Todo:     todo,
		}
	}
}

// DetectionWs 项目检测
func (srv *Service) DetectionWs(wsConn *websocket.Conn, spaceWithId *common.SpaceWithId) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), srv.detectionTimeout)
	dMsgChan := make(chan *DetectionMsg)
	defer close(dMsgChan)
	sendMsg := sendDetectionMsgFn(dMsgChan)

	//检测逻辑
	go func() {
		var err error
		defer func() {
			if _err := recover(); _err != nil {
				srv.log.Error("1.DetectionWs 已关闭写入渠道", zap.Any("_err", _err))
			} else {
				cancel()
			}
		}()
		defer func() {
			if err != nil {
				_ = wsConn.WriteMessage(websocket.TextMessage, []byte("error"))
			} else {
				_ = wsConn.WriteMessage(websocket.TextMessage, []byte("success"))
			}
		}()
		sendMsg("检测项目不存在", "请检查项目是否存在，或者刷新页面再尝试", "", 0)
		project := model.Project{}
		err = srv.db.Where(spaceWithId).Preload("Servers").First(&project).Error
		if err != nil {
			sendMsg("检测项目不存在", "请检查项目是否存在，或者刷新页面再尝试", "", 0)
			return
		}
		//clone项目代码
		_, err = srv.repo.New(repo.TypeRepo(project.RepoType), project.RepoUrl, strconv.Itoa(int(project.ID)))
		if err != nil {
			sendMsg("代码clone失败", "1、请检查仓库地址："+project.RepoUrl+"是否正确；\n 2、请检查"+project.RepoType+"相关配置是否正确", err.Error(), 0)
			return
		}
		if len(project.Servers) == 0 {
			sendMsg("项目未绑定发布服务器", "请添加发布服务器后，在修改项目重新选择绑定", "", 0)
			err = errors.New("项目未绑定发布服务器")
			return
		}
		g := sync.WaitGroup{}
		//检查服务器
		for _, server := range project.Servers {
			g.Add(1)
			go func(server model.Server) {
				defer func() {
					if _err := recover(); _err != nil {
						srv.log.Error("2.DetectionWs 已关闭写入渠道", zap.Any("_err", _err))
					}
					g.Done()
				}()
				buf := bytes.Buffer{}
				re, _err := srv.ssh.NewRemoteExec(ssh.ServerConfig{User: server.User, Port: server.Port, Host: server.Host}, &buf)
				if _err != nil {
					sendMsg("远程目标机器免密码登录失败",
						fmt.Sprintf("在宿主机中配置免密码登录，把宿主机用户[%s]的~/.ssh/id_rsa.pub添加到远程目标机器用户[%s]的~/.ssh/authorized_keys", server.User, server.User),
						_err.Error(),
						server.ID)
					err = _err
					return
				}
				defer func() { _ = re.Close() }()

				webroot := filepath.Dir(project.TargetRoot)
				cmd := fmt.Sprintf("[ -d %s ] || mkdir -p %s", webroot, webroot)
				_err = re.Run(cmd)
				if _err != nil {
					sendMsg("远程目标机器创建目录失败",
						fmt.Sprintf("请检查远程目标服务器用户[%s]的权限，失败执行命令：%s", server.User, cmd),
						_err.Error(),
						server.ID)
					err = _err
					return
				}

				cmd = fmt.Sprintf("[ -L \"%s\" ] && echo \"true\" || echo \"false\"", project.TargetRoot)
				buf.Reset()
				_err = re.Run(cmd)
				if _err != nil {
					sendMsg("目标机器执行命令失败",
						fmt.Sprintf("请检查远程目标服务器用户[%s]的权限，失败执行命令：%s", server.User, cmd),
						_err.Error(),
						server.ID)
					err = _err
					return
				}
				if buf.String() == "false" {
					sendMsg("远程目标机器webroot不能是已建好的目录",
						"手工删除远程目标机器："+server.Host+" webroot目录："+project.TargetRoot,
						"远程目标机器%s webroot不能是已存在的目录，必须为软链接，你不必新建，walle会自行创建。",
						server.ID)
					err = _err
					return
				}
			}(server)
		}
		g.Wait()
	}()

	//客户端发送消息
	var res []byte
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-dMsgChan:
			res, err = json.Marshal(msg)
			if err != nil {
				srv.log.Error("DetectionWs json.Marshal error", zap.Error(err))
				continue
			}
			err = wsConn.WriteMessage(websocket.TextMessage, res)
			if err != nil {
				cancel()
				return Error.New("DetectionWs wsConn.WriteMessage error:%s", err)
			}
		}
	}
}
