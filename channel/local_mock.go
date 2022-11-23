/*
 * Copyright 1999-2019 Alibaba Group Holding Ltd.
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

package channel

import (
	"context"
	"fmt"
	"github.com/chaosblade-io/chaosblade-spec-go/log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
)

// MockLocalChannel for testing
type MockLocalChannel struct {
	ScriptPath string
	// mock function
	RunFunc                     func(ctx context.Context, script, args string) *spec.Response
	GetPidsByProcessCmdNameFunc func(processName string, ctx context.Context) ([]string, error)
	GetPidsByProcessNameFunc    func(processName string, ctx context.Context) ([]string, error)
	GetPsArgsFunc               func(ctx context.Context) string
	IsCommandAvailableFunc      func(ctx context.Context, commandName string) bool
	ProcessExistsFunc           func(pid string) (bool, error)
	GetPidUserFunc              func(pid string) (string, error)
	GetPidsByLocalPortsFunc     func(ctx context.Context, localPorts []string) ([]string, error)
	GetPidsByLocalPortFunc      func(ctx context.Context, localPort string) ([]string, error)
}

func NewMockLocalChannel() spec.Channel {
	return &MockLocalChannel{
		ScriptPath:                  util.GetBinPath(),
		RunFunc:                     defaultRunFunc,
		GetPidsByProcessCmdNameFunc: defaultGetPidsByProcessCmdNameFunc,
		GetPidsByProcessNameFunc:    defaultGetPidsByProcessNameFunc,
		GetPsArgsFunc:               defaultGetPsArgsFunc,
		IsCommandAvailableFunc:      defaultIsCommandAvailableFunc,
		ProcessExistsFunc:           defaultProcessExistsFunc,
		GetPidUserFunc:              defaultGetPidUserFunc,
		GetPidsByLocalPortsFunc:     defaultGetPidsByLocalPortsFunc,
		GetPidsByLocalPortFunc:      defaultGetPidsByLocalPortFunc,
	}
}

func (l *MockLocalChannel) Name() string {
	return "mock"
}

func (mlc *MockLocalChannel) GetPidsByProcessCmdName(processName string, ctx context.Context) ([]string, error) {
	return mlc.GetPidsByProcessCmdNameFunc(processName, ctx)
}

func (mlc *MockLocalChannel) GetPidsByProcessName(processName string, ctx context.Context) ([]string, error) {
	return mlc.GetPidsByProcessNameFunc(processName, ctx)
}

func (mlc *MockLocalChannel) GetPsArgs(ctx context.Context) string {
	return mlc.GetPsArgsFunc(ctx)
}

func (mlc *MockLocalChannel) IsAlpinePlatform(ctx context.Context) bool {
	return false
}
func (mlc *MockLocalChannel) IsAllCommandsAvailable(ctx context.Context, commandNames []string) (*spec.Response, bool) {
	return nil, false
}

func (mlc *MockLocalChannel) IsCommandAvailable(ctx context.Context, commandName string) bool {
	return mlc.IsCommandAvailableFunc(ctx, commandName)
}

func (mlc *MockLocalChannel) ProcessExists(pid string) (bool, error) {
	return mlc.ProcessExistsFunc(pid)
}

func (mlc *MockLocalChannel) GetPidUser(pid string) (string, error) {
	return mlc.GetPidUserFunc(pid)
}

func (mlc *MockLocalChannel) GetPidsByLocalPorts(ctx context.Context, localPorts []string) ([]string, error) {
	return mlc.GetPidsByLocalPortsFunc(ctx, localPorts)
}

func (mlc *MockLocalChannel) GetPidsByLocalPort(ctx context.Context, localPort string) ([]string, error) {
	return mlc.GetPidsByLocalPortFunc(ctx, localPort)
}

func (mlc *MockLocalChannel) Run(ctx context.Context, script, args string) *spec.Response {
	return mlc.RunFunc(ctx, script, args)
}

func (mlc *MockLocalChannel) GetScriptPath() string {
	return mlc.ScriptPath
}

func (mlc *MockLocalChannel) RunScript(ctx context.Context, script, args, uid string) *spec.Response {
	pid := ctx.Value(NSTargetFlagName)
	if pid == nil {
		return spec.ResponseFailWithFlags(spec.CommandIllegal, script)
	}

	ns_script := fmt.Sprintf("-t %s", pid)

	if ctx.Value(NSPidFlagName) == spec.True {
		ns_script = fmt.Sprintf("%s -p", ns_script)
	}

	if ctx.Value(NSMntFlagName) == spec.True {
		ns_script = fmt.Sprintf("%s -m", ns_script)
	}

	if ctx.Value(NSNetFlagName) == spec.True {
		ns_script = fmt.Sprintf("%s -n", ns_script)
	}

	isBladeCommand := isBladeCommand(script)
	if isBladeCommand && !util.IsExist(script) {
		// TODO nohup invoking
		return spec.ResponseFailWithFlags(spec.ChaosbladeFileNotFound, script)
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	//main.tar是一个或者多个文件直接打的tar，外层没有目录，eg: scriptFile="/Users/apple/tar_file/main.tar
	tarDistDir := filepath.Dir(script) + "/" + fmt.Sprintf("%d", time.Now().UnixNano())
	UnTar(script, tarDistDir)
	//判断有没有main主文件，没有直接返错误
	scriptMain := tarDistDir + "/main"
	if _, err := os.Stat(scriptMain); os.IsNotExist(err) {
		outMessage := " script files must contain main file " + err.Error()
		return spec.ResponseFailWithFlags(spec.FileNotExist, outMessage)
	}

	ns_script = fmt.Sprintf("%s -- /bin/sh -c", ns_script)

	programPath := util.GetProgramPath()
	if path.Base(programPath) != spec.BinPath {
		programPath = path.Join(programPath, spec.BinPath)
	}
	bin := path.Join(programPath, spec.NSExecBin)
	log.Debugf(ctx, `Command: %s %s "%s"`, bin, ns_script, args)

	//cmdChmod := exec.Command("sh", "-c", "chmod 777 "+scriptMain)
	cmdChmod := exec.CommandContext(timeoutCtx, bin, "chmod 777 "+scriptMain)
	outputChmod, err := cmdChmod.CombinedOutput()
	outMsgChmod := string(outputChmod)
	log.Debugf(ctx, "Command Result, outputChmod: %v, err: %v", outMsgChmod, err)
	if err != nil {
		outMsgChmod += " " + err.Error()
		return spec.ResponseFailWithFlags(spec.OsCmdExecFailed, cmdChmod, outMsgChmod)
	}
	//录制script脚本执行过程
	time := "/tmp/" + uid + ".time"
	out := "/tmp/" + uid + ".out"
	if runtime.GOOS == "darwin" {
		scriptMain = "script  -t 2>" + time + " -a " + out + " " + scriptMain
		if args != "" {
			args = scriptMain + " " + args
		} else {
			args = scriptMain
		}

	} else {
		scriptMain = "script  -t 2>" + time + " -a " + out + "  -c  " + "\"" + scriptMain
		if args != "" {
			args = scriptMain + " " + args
		} else {
			args = scriptMain
		}
		args += "\""
	}

	split := strings.Split(ns_script, " ")

	cmd := exec.CommandContext(timeoutCtx, bin, append(split, args)...)
	output, err := cmd.CombinedOutput()
	outMsg := string(output)
	log.Debugf(ctx, "Command Result, output: %v, err: %v", outMsg, err)
	// TODO shell-init错误
	if strings.TrimSpace(outMsg) != "" {
		resp := spec.Decode(outMsg, nil)
		if resp.Code != spec.ResultUnmarshalFailed.Code {
			return resp
		}
	}
	if err == nil {
		return spec.ReturnSuccess(outMsg)
	}
	outMsg += " " + err.Error()
	return spec.ResponseFailWithFlags(spec.OsCmdExecFailed, cmd, outMsg)
}

var defaultGetPidsByProcessCmdNameFunc = func(processName string, ctx context.Context) ([]string, error) {
	return []string{}, nil
}
var defaultGetPidsByProcessNameFunc = func(processName string, ctx context.Context) ([]string, error) {
	return []string{}, nil
}
var defaultGetPsArgsFunc = func(ctx context.Context) string {
	return "-eo user,pid,ppid,args"
}
var defaultIsCommandAvailableFunc = func(ctx context.Context, commandName string) bool {
	return false
}
var defaultProcessExistsFunc = func(pid string) (bool, error) {
	return false, nil
}
var defaultGetPidUserFunc = func(pid string) (string, error) {
	return "admin", nil
}
var defaultGetPidsByLocalPortsFunc = func(ctx context.Context, localPorts []string) ([]string, error) {
	return []string{}, nil
}
var defaultGetPidsByLocalPortFunc = func(ctx context.Context, localPort string) ([]string, error) {
	return []string{}, nil
}
var defaultRunFunc = func(ctx context.Context, script, args string) *spec.Response {
	return spec.ReturnSuccess("success")
}
