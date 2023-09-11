package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wuzfei/go-helper/compress"
	"github.com/wuzfei/go-helper/files"
	"github.com/zeebo/errs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"yema.dev/app/internal/bytes"
	"yema.dev/app/model"
	"yema.dev/app/pkg/repo"
	"yema.dev/app/pkg/ssh"
	"yema.dev/app/utils"
)

var ErrStopDeploy = Error.New("终止发布任务")

var localServerId = int64(0)

type RemoteErrs struct {
	sync.Map
}

func (r RemoteErrs) Error() string {
	res := ""
	r.Range(func(key, value any) bool {
		res = fmt.Sprintf("[%d]%s;%s", key, value, res)
		return true
	})
	return res
}

type deployDirs struct {
	localWarehouseDir, //发布时本地代码临时目录
	localCodePackage, //发布时本地代码压缩包全路径名称
	remoteReleaseDir, //远程对应版本的代码或程序目录
	remoteReleasePackage, //远程发布程序目录
	remoteRootLink string //远程发布程序软连接，比如nginx将指向此地址
}

func (dd *deployDirs) Remove() error {
	//移除本地目录
	err := errs.Combine(
		os.RemoveAll(dd.localWarehouseDir),
		os.RemoveAll(dd.localCodePackage))
	return err
}

func (dd *deployDirs) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("localWarehouseDir", dd.localWarehouseDir)
	enc.AddString("localCodePackage", dd.localCodePackage)
	enc.AddString("remoteReleaseDir", dd.remoteReleaseDir)
	enc.AddString("remoteReleasePackage", dd.remoteReleasePackage)
	enc.AddString("remoteRootLink", dd.remoteRootLink)
	return nil
}

type step struct {
	step   int8
	status int8
}

type Task struct {
	db   *gorm.DB
	log  *zap.Logger
	ssh  *ssh.Ssh
	repo *repo.Repos

	userId         int64 //操作人员
	model          *model.Task
	ReleaseTimeout time.Duration

	started    bool
	deployDirs *deployDirs

	doneError chan error

	steps map[int64]*step

	taskLogs map[int64]*bytes.BufferOver
}

func NewTask(taskModel *model.Task, db *gorm.DB, log *zap.Logger, ssh *ssh.Ssh, repo *repo.Repos) (*Task, error) {
	taskLogs := make(map[int64]*bytes.BufferOver)
	steps := make(map[int64]*step)
	for i := range taskModel.Servers {
		taskLogs[taskModel.Servers[i].ID] = bytes.NewBufferOver()
		steps[taskModel.Servers[i].ID] = &step{}
	}
	taskLogs[localServerId] = bytes.NewBufferOver()
	steps[localServerId] = &step{}
	return &Task{
		db:   db,
		log:  log,
		ssh:  ssh,
		repo: repo,

		doneError: make(chan error),
		model:     taskModel,
		taskLogs:  taskLogs,
		steps:     steps,
	}, nil
}

func (t *Task) Run(ctx context.Context) error {
	err := t.Start(ctx)
	if err != nil {
		return err
	}
	return t.Wait()
}

func (t *Task) Start(ctx context.Context) (err error) {
	if t.started {
		return Error.New("deploy task already started")
	}
	//检查基本状态
	err = t.check()
	if err != nil {
		return Error.Wrap(err)
	}

	t.started = true

	//更新发布状态和版本
	t.model.Status = model.TaskStatusRelease
	t.model.Version = t.createReleaseVersion()
	err = t.db.Model(model.Task{}).Where("id = ? and status=?", t.model.ID, model.TaskStatusAudit).
		Select("status", "Version").UpdateColumns(t.model).Error
	if err != nil {
		return Error.Wrap(err)
	}

	go func() {
		t.start(ctx)
	}()
	return
}

// prevDeploy step1.检出代码前置操作
func (t *Task) prevDeploy(ctx context.Context) (err error) {
	//1、检查仓库，
	t.steps[localServerId].step = 1
	defer func() {
		if err != nil {
			t.steps[localServerId].status = 2
		} else {
			t.steps[localServerId].status = 1
		}
	}()
	t.log.Debug("1.1、检查仓库")
	_repo, err := t.getRepo()
	if err != nil {
		return errors.New("获取代码仓库错误：" + err.Error())
	}
	localDeployDir := filepath.Dir(_repo.Path())
	//发布压缩包名
	packageName := t.model.Version + ".tar.gz"
	t.deployDirs = &deployDirs{
		localWarehouseDir:    filepath.Join(localDeployDir, t.model.Version),
		localCodePackage:     filepath.Join(localDeployDir, packageName),
		remoteReleaseDir:     filepath.Join(t.model.Project.TargetReleases, t.model.Version),
		remoteReleasePackage: filepath.Join(t.model.Project.TargetReleases, packageName),
		remoteRootLink:       t.model.Project.TargetRoot,
	}
	//2、执行用户打包前命令
	t.log.Debug("1.2、执行用户打包前命令")
	commands := parseCommands(t.model.Project.PrevDeploy)
	for _, cmd := range commands {
		r := t.newRecordLocal(cmd, t.envs())
		if err = r.Run(ctx); err != nil {
			return err
		}
	}
	return nil
}

// deploy step2.检出代码
func (t *Task) deploy(ctx context.Context) (err error) {
	t.steps[localServerId].step = 2
	defer func() {
		if err != nil {
			t.steps[localServerId].status = 2
		} else {
			t.steps[localServerId].status = 1
		}
	}()
	//1、检出代码
	t.log.Debug("2.1、检出代码")
	_repo, err := t.getRepo()
	if err != nil {
		err = errors.New("获取代码仓库错误：" + err.Error())
		return
	}
	if t.model.Tag != "" {
		err = _repo.CheckoutToTag(t.model.Tag)
	} else if t.model.Branch != "" && t.model.CommitId != "" {
		err = _repo.CheckoutToCommit(t.model.Branch, t.model.CommitId)
	} else {
		err = errors.New("发布分支选取错误")
	}
	if err != nil {
		return err
	}
	//2、复制发布版本代码到新目录，以便下面执行编译等操作
	t.log.Debug("2.2、复制发布版本代码到新目录，以便下面执行编译等操作")
	if _, err = files.CopyDirToDir(t.deployDirs.localWarehouseDir, _repo.Path()); err != nil {
		err = errors.New("检出代码失败：" + err.Error())
		return
	}
	return nil
}

// postDeploy step3.推送到服务器前的操作，比如下载依赖，编译等
func (t *Task) postDeploy(ctx context.Context) (err error) {
	t.steps[localServerId].step = 3
	defer func() {
		if err != nil {
			t.steps[localServerId].status = 2
		} else {
			t.steps[localServerId].status = 1
		}
	}()
	//1、在检出代码执行用户发布前命令
	t.log.Debug("3.1、在检出代码执行用户发布前命令")
	commands := parseCommands(t.model.Project.PostDeploy)
	for _, cmd := range commands {
		cmd = fmt.Sprintf("cd %s && %s", t.deployDirs.localWarehouseDir, cmd)
		r := t.newRecordLocal(cmd, t.envs())
		if err = r.Run(ctx); err != nil {
			return err
		}
	}
	//2、打包代码
	t.log.Debug("3.2、打包代码")
	cmd := fmt.Sprintf("tar -zcvf %s -C %s", t.deployDirs.localCodePackage, t.deployDirs.localWarehouseDir)
	record := t.newRecordLocal(cmd, nil)
	record.SetSaveTime()
	err = compress.PackMatch(t.deployDirs.localCodePackage, t.deployDirs.localWarehouseDir, t.getFileMatch())
	if err != nil {
		_ = record.Save(254, "打包代码出错:"+err.Error())
		return err
	}
	_ = record.Save(0, "success")
	return nil
}

func (t *Task) remoteRelease(ctx context.Context) error {
	remoteErrs := RemoteErrs{}
	wg := sync.WaitGroup{}
	for _, s := range t.model.Servers {
		wg.Add(1)
		go func(server model.Server) {
			remoteErrs.Store(server.ID, t.remoteRun(ctx, &server))
			wg.Done()
		}(s)
	}
	wg.Wait()
	return remoteErrs
}

// remoteRun 远程服务器执行部署
func (t *Task) remoteRun(ctx context.Context, server *model.Server) error {
	for _, f := range []func(ctx2 context.Context, server *model.Server) error{t.prevRelease, t.release, t.postRelease} {
		select {
		case <-ctx.Done():
			return ErrStopDeploy
		default:
			if err := f(ctx, server); err != nil {
				return err
			}
		}
	}
	return nil
}

// prevRelease step4.推送代码到服务器前的操作
func (t *Task) prevRelease(ctx context.Context, server *model.Server) (err error) {
	t.steps[server.ID].step = 4
	defer func() {
		if err != nil {
			t.steps[server.ID].status = 2
		} else {
			t.steps[server.ID].status = 1
		}
	}()
	//解压程序包
	//_tarCmd := fmt.Sprintf("mkdir -p %s ", filepath.Dir(t.deployDirs.remoteReleasePackage))
	//r := NewRecord(model.RecordTypePrevRelease, t.model.ID, t.userId, _tarCmd, server, t.envs())
	//if err := r.Run(t.ctx); err != nil {
	//	return err
	//}
	//1、上传程序包
	t.log.Debug("4.1、上传程序包", zap.String("server", server.Hostname()))
	_saveCmd := fmt.Sprintf("scp -P%d %s@%s:%s %s:%s", server.Port, utils.CurrentUser.Username, utils.CurrentHostname, t.deployDirs.localCodePackage, server.Hostname(), t.deployDirs.remoteReleasePackage)
	record := t.newRecordRemote(_saveCmd, server, nil)
	record.SetSaveTime()
	sftp, err := t.ssh.NewSftp(ssh.ServerConfig{Host: server.Host, User: server.User, Port: server.Port})
	if err == nil {
		err = sftp.Copy(t.deployDirs.localCodePackage, t.deployDirs.remoteReleasePackage)
		sftp.Close()
	}
	if err != nil {
		_ = record.Save(254, "上传程序出错:"+err.Error())
		return err
	}
	_ = record.Save(0, "success")

	//2、解压程序包
	t.log.Debug("4.2、在服务器解压程序包", zap.String("server", server.Hostname()))
	_tarCmd := fmt.Sprintf("mkdir -p %s && tar -zxvf %s -C %s", t.deployDirs.remoteReleaseDir, t.deployDirs.remoteReleasePackage, t.deployDirs.remoteReleaseDir)
	r := t.newRecordRemote(_tarCmd, server, t.envs())
	if err = r.Run(ctx); err != nil {
		return err
	}
	//3、执行用户命令
	t.log.Debug("4.3、执行用户命令", zap.String("server", server.Hostname()))
	commands := parseCommands(t.model.Project.PrevRelease)
	for _, cmd := range commands {
		cmd = fmt.Sprintf("cd %s && %s", t.deployDirs.remoteReleaseDir, cmd)
		r = t.newRecordRemote(cmd, server, t.envs())
		if err = r.Run(ctx); err != nil {
			return err
		}
	}
	return nil
}

// release step5.部署程序
func (t *Task) release(ctx context.Context, server *model.Server) (err error) {
	t.steps[server.ID].step = 5
	defer func() {
		if err != nil {
			t.steps[server.ID].status = 2
		} else {
			t.steps[server.ID].status = 1
		}
	}()
	//1、获取上一个部署版本，保存下来
	t.log.Debug("5.1、获取上一个部署版本，保存下来", zap.String("server", server.Hostname()))
	cmd := fmt.Sprintf("[ -L %s ] && readlink %s || echo \"\"", t.deployDirs.remoteRootLink, t.deployDirs.remoteRootLink)
	record := t.newRecordRemote(cmd, server, t.envs())
	if err = record.Run(ctx); err != nil {
		return err
	}
	t.model.PrevVersion = record.Output()

	//2、部署代码，创建并替换源软连接
	t.log.Debug("5.2、部署代码，创建并替换源软连接", zap.String("server", server.Hostname()))
	tmpLink := fmt.Sprintf("%s_tmp", t.deployDirs.remoteRootLink)
	cmd = fmt.Sprintf("mkdir -p %s && ln -sfn %s %s", filepath.Dir(t.deployDirs.remoteRootLink), t.deployDirs.remoteReleaseDir, tmpLink)
	record = t.newRecordRemote(cmd, server, t.envs())
	if err = record.Run(ctx); err != nil {
		return err
	}

	t.log.Debug("5.3、更新数据库记录", zap.String("server", server.Hostname()))
	cmd = fmt.Sprintf("mv -fT %s %s", tmpLink, t.deployDirs.remoteRootLink)
	record = t.newRecordRemote(cmd, server, t.envs())
	if err = record.Run(ctx); err != nil {
		return err
	}
	t.db.Select("prev_version").UpdateColumns(t.model)
	return nil
}

// postRelease 6、执行部署完成功后用户相关命令
func (t *Task) postRelease(ctx context.Context, server *model.Server) (err error) {
	t.steps[server.ID].step = 6
	defer func() {
		if err != nil {
			t.steps[server.ID].status = 2
		} else {
			t.steps[server.ID].status = 1
		}
	}()
	t.log.Debug("6.1、执行部署完成功后用户相关命令", zap.String("server", server.Hostname()))
	commands := parseCommands(t.model.Project.PostRelease)
	for _, cmd := range commands {
		cmd = fmt.Sprintf("cd %s && %s", t.deployDirs.remoteRootLink, cmd)
		r := t.newRecordRemote(cmd, server, t.envs())
		if err = r.Run(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (t *Task) start(ctx context.Context) {
	var err error
loopFor:
	for _, f := range []func(ctx2 context.Context) error{t.prevDeploy, t.deploy, t.postDeploy, t.remoteRelease} {
		select {
		case <-ctx.Done():
			err = ErrStopDeploy
			break loopFor
		default:
			err = f(ctx)
			if err != nil {
				break loopFor
			}
		}
	}
	t.doneError <- err
}

// Output 发布日志即时输出
func (t *Task) Output(ctx context.Context) <-chan *ConsoleMsg {
	msg := make(chan *ConsoleMsg, len(t.model.Servers))
	go func() {
		defer close(msg)
		offsetMap := make(map[int64]int)
		for k := range t.taskLogs {
			offsetMap[k] = 0
		}
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			for k, off := range offsetMap {
				_data := make([]byte, 360)
				n, err := t.taskLogs[k].ReadAt(_data, off)
				offsetMap[k] += n
				_data = _data[:n]
				if n > 0 {
					msg <- &ConsoleMsg{
						Step:     t.steps[k].step,
						Status:   t.steps[k].status,
						ServerId: k,
						Data:     string(_data),
					}
				}
				if errors.Is(err, io.EOF) {
					delete(offsetMap, k)
				}
			}
			if len(offsetMap) == 0 {
				break
			} else {
				time.Sleep(time.Second / 50)
			}
		}
	}()
	return msg
}

func (t *Task) Wait() error {
	doneErr := <-t.doneError
	close(t.doneError)
	for i := range t.taskLogs {
		t.taskLogs[i].WriteOver()
	}

	t.model.Status = model.TaskStatusFinish
	if doneErr != nil {
		t.model.LastError = doneErr.Error()
		t.model.Status = model.TaskStatusReleaseFail
		updates := make([]*model.TaskServer, 0)
		if re, ok := doneErr.(RemoteErrs); ok {
			t.model.Status = model.TaskStatusReleasePartFail
			re.Range(func(key, value any) bool {
				if value != nil {
					updates = append(updates, &model.TaskServer{
						TaskId:   t.model.ID,
						ServerId: key.(int64),
						Status:   model.TaskServerStatusFail,
						Err:      value.(error).Error(),
					})
				} else {
					updates = append(updates, &model.TaskServer{
						TaskId:   t.model.ID,
						ServerId: key.(int64),
						Status:   model.TaskServerStatusSuccess,
					})
				}
				return true
			})
		} else {
			for _, s := range t.model.Servers {
				updates = append(updates, &model.TaskServer{
					TaskId:   t.model.ID,
					ServerId: s.ID,
					Status:   model.TaskServerStatusSuccess,
				})
			}
		}
		_err := t.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "task_id"}, {Name: "server_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "err"}),
		}).Create(&updates).Error
		if _err != nil {
			_s, _ := json.Marshal(updates)
			t.log.Debug("更新服务器部署状态出错", zap.ByteString("updates", _s), zap.Error(_err))
		}
	}
	mb, _ := json.Marshal(t.model)

	err := t.deployDirs.Remove()
	if err != nil {
		t.log.Error("发布完成移除临时文件目录出错", zap.Error(err), zap.Object("deployDirs", t.deployDirs))
	}

	if err = t.db.Model(model.Task{}).
		Select("status", "last_error").Where("id = ?", t.model.ID).UpdateColumns(t.model).Error; err != nil {
		t.log.Error("部署完成，更新数据库时出错", zap.ByteString("task_model", mb), zap.Error(doneErr), zap.Error(err))
	} else {
		t.log.Debug("部署完成", zap.ByteString("task_model", mb))
	}
	return doneErr
}

func (t *Task) envs() *ssh.Envs {
	_envs := ssh.NewEnvsBySliceKV(parseCommands(t.model.Project.TaskVars))
	//_envs := NewEnvs()
	_envs.Add("PROJECT_ID", t.model.Project.ID)
	_envs.Add("PROJECT_NAME", t.model.Project.Name)
	_envs.Add("TASK_ID", t.model.ID)
	_envs.Add("TASK_NAME", t.model.Name)
	//_envs.Add("DEPLOY_PATH", t.deployPath)
	_envs.Add("RELEASE_PATH", &t.model.Project.TargetRoot)
	return _envs
}

func (t *Task) getFileMatch() compress.Match {
	_files := strings.TrimSpace(t.model.Project.Excludes)
	if _files != "" {
		regs := make([]string, 0)
		for _, v := range strings.Split(_files, "\n") {
			v = strings.TrimSpace(v)
			if v != "" {
				regs = append(regs, v)
			}
		}
		if len(regs) == 0 {
			return nil
		}
		if t.model.Project.IsInclude == model.ProjectIsInclude {
			return compress.FileMatch(regs...)
		}
		return compress.ReFileMatch(regs...)
	}
	return nil
}

func (t *Task) getRepo() (repo.Repo, error) {
	fmt.Println(repo.TypeRepo(t.model.Project.RepoType), t.model.Project.RepoUrl, fmt.Sprintf("%d", t.model.Project.ID))
	return t.repo.New(repo.TypeRepo(t.model.Project.RepoType), t.model.Project.RepoUrl, fmt.Sprintf("%d", t.model.Project.ID))
}

func (t *Task) newRecordLocal(cmd string, envs *ssh.Envs) *Record {
	fmt.Println("newRecordLocal", cmd)
	t.taskLogs[localServerId].Write([]byte(utils.CurrentHost + " $ " + cmd + "\r\n"))
	if envs == nil {
		envs = ssh.NewEnvs()
	}
	return NewRecordLocal(t.db, t.log, t.ssh, t.model.ID, t.userId, cmd, envs, t.taskLogs[localServerId])
}

func (t *Task) newRecordRemote(cmd string, server *model.Server, envs *ssh.Envs) *Record {
	fmt.Println("newRecordRemote", cmd)
	t.taskLogs[server.ID].Write([]byte(server.Hostname() + " $ " + cmd + "\r\n"))
	if envs == nil {
		envs = ssh.NewEnvs()
	}
	return NewRecordRemote(t.db, t.log, t.ssh, t.model.ID, t.userId, cmd, server, envs, t.taskLogs[server.ID])
}

func (t *Task) createReleaseVersion() string {
	return fmt.Sprintf("%d_%d_%s", t.model.Project.ID, t.model.ID, time.Now().Format("20060102_150405"))
}

// parseCommands 解析命令，支持'#'，'//'的行注释
func parseCommands(commands string) []string {
	res := make([]string, 0)
	commands = strings.TrimSpace(commands)
	if commands == "" {
		return res
	}
	arr := strings.Split(commands, "\n")
	for _, v := range arr {
		v = strings.TrimSpace(v)
		if v == "" || v[:1] == "#" || (len(v) > 1 && v[:2] == "//") {
			continue
		}
		res = append(res, v)
	}
	return res
}

// check 检查基本状态是否可以发布上线
func (t *Task) check() error {
	if t.model.Status != model.TaskStatusAudit {
		return errors.New("任务未处于审核通过状态，无法发布")
	}
	if !t.model.Environment.Status.IsEnable() {
		return fmt.Errorf("该环境[%s]已经禁止发版，请联系相关负责人处理", t.model.Environment.Name)
	}
	if !t.model.Project.Status.IsEnable() {
		return fmt.Errorf("该项目[%s]已经禁止发版，请联系相关负责人处理", t.model.Project.Name)
	}
	if len(t.model.Servers) == 0 {
		return fmt.Errorf("该任务[%s]发布服务器为空，请联系相关负责人处理", t.model.Name)
	}
	return nil
}
