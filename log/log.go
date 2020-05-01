// Copyright © 2020 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package log

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// Specifies the log levels
const (
	LevelError = iota + 1
	LevelWarning
	LevelInfo
	LevelDebug
	LevelVerbose // This is the same as Debug, but without the repeat filtering
)

// Specifies the command tags
const (
	Mixer       = "MIXER"
	Dnf         = "DNF"
	Rpm2Archive = "RPM2ARCHIVE"
	Tar         = "TAR"
	BsDiff      = "BSDIFF"
	Mca         = "MCA"
	Ssl         = "SSL"
	Git         = "GIT"
	CreateRepo  = "CREATEREPO"
	Installer   = "INSTALLER"
)

var (
	level      = LevelDebug
	levelMap   = map[int]string{}
	fileHandle *os.File
	logging    = false
	lineLast   string
	lineCount  int
	cmdMap     = map[string]bool{}
)

func init() {
	levelMap[LevelError] = "ERROR"
	levelMap[LevelWarning] = "WARNING"
	levelMap[LevelInfo] = "INFO"
	levelMap[LevelDebug] = "DEBUG"
	levelMap[LevelVerbose] = "VERBOSE"
	cmdMap[Mixer] = true
	cmdMap[Dnf] = true
	cmdMap[Rpm2Archive] = true
	cmdMap[Tar] = true
	cmdMap[BsDiff] = true
	cmdMap[Mca] = true
	cmdMap[Ssl] = true
	cmdMap[Git] = true
	cmdMap[CreateRepo] = true
	cmdMap[Installer] = true
}

// SetLogLevel sets the default log level to l
func SetLogLevel(l int) {
	if l < LevelError {
		level = LevelError
		logTag("WRN", Mixer, "Log Level '%d' too low, forcing to %s (%d)", l, levelMap[level], level)
	} else if l > LevelVerbose {
		level = LevelVerbose
		logTag("WRN", Mixer, "Log Level '%d' too high, forcing to %s (%d)", l, levelMap[level], level)
	} else {
		level = l
		Debug(Mixer, "Log Level set to %s (%d)", levelMap[level], l)
	}
}

// SetOutputFilename ... sets the default log output to filename instead of stdout/stderr
func SetOutputFilename(logFile string) (*os.File, error) {
	var err error
	if _, err = os.Stat(logFile); os.IsNotExist(err) {
		fileHandle, err = os.OpenFile(logFile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			return nil, err
		}

		err = fileHandle.Chmod(0666)
		if err != nil {
			return nil, err
		}
	} else {
		fileHandle, err = os.OpenFile(logFile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			return nil, err
		}
	}

	log.SetOutput(fileHandle)
	logging = true
	return fileHandle, nil
}

// CloseLogHandler ... closes the file handler of logging file
func CloseLogHandler() {
	var err error
	if logging {
		err = fileHandle.Close()
		if err != nil {
			fmt.Printf("WARNING: couldn't close file for log: %s\n", err)
		}
	}
}

func logTag(tag string, cmdTag, format string, a ...interface{}) {
	// If there are no variable to pass to the format,
	// then we can escape any % signs.
	if len(a) < 1 {
		format = strings.ReplaceAll(format, "%", "%%")
	}

	f := "[" + tag + "]" + "[" + cmdTag + "] " + format + "\n"
	output := fmt.Sprintf(f, a...)

	if level >= LevelVerbose {
		log.Print(output)
		return
	}

	if output != lineLast {
		// output the previous repeated line
		if lineCount > 0 {
			plural := ""
			if lineCount > 1 {
				plural = "s"
			}

			repeat := fmt.Sprintf("[%s] [Previous line repeated %d time%s]\n", tag, lineCount, plural)
			log.Print(repeat)
		}

		log.Print(output)

		lineLast = output
		lineCount = 0
	} else { // Repeated line
		lineCount++
	}
}

// Debug prints a debug log entry with DBG tag
func Debug(cmdTag, format string, a ...interface{}) {
	if level < LevelDebug || !logging {
		return
	}
	if _, ok := cmdMap[cmdTag]; !ok {
		cmdTag = Mixer
	}
	logTag("DBG", cmdTag, format, a...)
}

// Error prints an error log entry with ERR tag
func Error(cmdTag, format string, a ...interface{}) {
	fmt.Printf("Error: "+format+"\n", a...)
	if !logging {
		return
	}
	if _, ok := cmdMap[cmdTag]; !ok {
		cmdTag = Mixer
	}
	logTag("ERR", cmdTag, format, a...)
}

// Info prints an info log entry with INF tag
func Info(cmdTag, format string, a ...interface{}) {
	fmt.Printf(format+"\n", a...)
	if level < LevelInfo || !logging {
		return
	}
	if _, ok := cmdMap[cmdTag]; !ok {
		cmdTag = Mixer
	}
	logTag("INF", cmdTag, format, a...)
}

// Warning prints an warning log entry with WRN tag
func Warning(cmdTag, format string, a ...interface{}) {
	fmt.Printf("Warning: "+format+"\n", a...)
	if level < LevelWarning || !logging {
		return
	}
	if _, ok := cmdMap[cmdTag]; !ok {
		cmdTag = Mixer
	}
	logTag("WRN", cmdTag, format, a...)
}

// Verbose prints a verbose log entry with VRB tag
func Verbose(cmdTag, format string, a ...interface{}) {
	if level < LevelVerbose || !logging {
		return
	}
	if _, ok := cmdMap[cmdTag]; !ok {
		cmdTag = Mixer
	}
	logTag("VRB", cmdTag, format, a...)
}
