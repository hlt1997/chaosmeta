/*
 * Copyright 2022-2023 Chaos Meta Authors.
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

package diskio

import (
	"fmt"
	"github.com/ChaosMetaverse/chaosmetad/pkg/injector"
	"github.com/ChaosMetaverse/chaosmetad/pkg/log"
	"github.com/ChaosMetaverse/chaosmetad/pkg/utils"
	"github.com/ChaosMetaverse/chaosmetad/pkg/utils/cmdexec"
	"github.com/ChaosMetaverse/chaosmetad/pkg/utils/filesys"
	"github.com/ChaosMetaverse/chaosmetad/pkg/utils/process"
	"github.com/spf13/cobra"
	"os"
)

//TODO: It needs to be stated in the document that the target directory has at least 1G disk space remaining

func init() {
	injector.Register(TargetDiskIO, FaultDiskIOBurn, func() injector.IInjector { return &BurnInjector{} })
}

type BurnInjector struct {
	injector.BaseInjector
	Args    BurnArgs
	Runtime BurnRuntime
}

type BurnArgs struct {
	Mode  string `json:"mode"`
	Block string `json:"block"`
	Dir   string `json:"dir"`
}

type BurnRuntime struct {
}

func (i *BurnInjector) GetArgs() interface{} {
	return &i.Args
}

func (i *BurnInjector) GetRuntime() interface{} {
	return &i.Runtime
}

func (i *BurnInjector) SetDefault() {
	i.BaseInjector.SetDefault()

	if i.Args.Dir == "" {
		i.Args.Dir = DefaultDir
	}

	if i.Args.Mode == "" {
		i.Args.Mode = ModeRead
	}

	if i.Args.Block == "" {
		i.Args.Block = DefaultBlockSize
	}
}

func (i *BurnInjector) SetOption(cmd *cobra.Command) {
	//// i.BaseInjector.SetOption(cmd)
	cmd.Flags().StringVarP(&i.Args.Mode, "mode", "m", "", fmt.Sprintf("disk IO mode, support: %s、%s（default %s）", ModeRead, ModeWrite, ModeRead))
	cmd.Flags().StringVarP(&i.Args.Block, "block", "b", "", fmt.Sprintf("disk IO block size（default %s）, support unit: KB/MB（default KB）", DefaultBlockSize))
	cmd.Flags().StringVarP(&i.Args.Dir, "dir", "d", "", fmt.Sprintf("disk IO burn directory（default %s）", DefaultDir))
}

func (i *BurnInjector) getFileName() string {
	return fmt.Sprintf("%s/%s_%s", i.Args.Dir, DiskIOBurnFile, i.Info.Uid)
}

func (i *BurnInjector) Validator() error {
	if i.Args.Dir == "" {
		return fmt.Errorf("\"dir\" is empty")
	}

	if err := filesys.CheckDir(i.Args.Dir); err != nil {
		return fmt.Errorf("\"dir\"[%s] check error: %s", i.Args.Dir, err.Error())
	}

	if i.Args.Mode != ModeRead && i.Args.Mode != ModeWrite {
		return fmt.Errorf("\"mode\" not support %s, only support: %s、%s", i.Args.Mode, ModeRead, ModeWrite)
	}

	kbytes, _, err := utils.GetBlockKbytes(i.Args.Block)
	if err != nil {
		return fmt.Errorf("\"block\"[%s] is invalid: %s", i.Args.Block, err.Error())
	}

	if kbytes <= 0 || kbytes > MaxBlockK {
		return fmt.Errorf("\"block\"[%s] value must be in (0, 1G]", i.Args.Block)
	}

	return i.BaseInjector.Validator()
}

func (i *BurnInjector) Inject() error {
	var timeout int64
	if i.Info.Timeout != "" {
		timeout, _ = utils.GetTimeSecond(i.Info.Timeout)
	}

	blockK, stdStr, _ := utils.GetBlockKbytes(i.Args.Block)
	count := MaxBlockK / blockK
	if _, err := cmdexec.StartBashCmdAndWaitPid(fmt.Sprintf("%s %s %s %s %s %d %s %d",
		utils.GetToolPath(DiskIOBurnKey), i.Info.Uid, i.getFileName(), i.Args.Mode, stdStr, count, FlagDirect, timeout)); err != nil {
		if err := i.Recover(); err != nil {
			log.WithUid(i.Info.Uid).Warnf("undo error: %s", err.Error())
		}

		return err
	}

	return nil
}

func (i *BurnInjector) DelayRecover(timeout int64) error {
	return nil
}

func (i *BurnInjector) Recover() error {
	if i.BaseInjector.Recover() == nil {
		return nil
	}

	processKey := fmt.Sprintf("%s %s", DiskIOBurnKey, i.Info.Uid)
	isProExist, err := process.ExistProcessByKey(processKey)
	if err != nil {
		return fmt.Errorf("check process exist by key[%s] error: %s", processKey, err.Error())
	}

	if isProExist {
		if err := process.KillProcessByKey(processKey, process.SIGKILL); err != nil {
			return fmt.Errorf("kill process by key[%s] error: %s", processKey, err.Error())
		}
	}

	file := i.getFileName()
	isFileExist, err := filesys.ExistPath(file)
	if err != nil {
		return fmt.Errorf("check file[%s] exist error: %s", file, err.Error())
	}

	if isFileExist {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("remove file[%s] error: %s", file, err.Error())
		}
	}

	return nil
}
