package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/dashjay/baize/pkg/caches"
)

func main() {
	t := flag.String("t", "", "type of file path")
	file := flag.String("f", "", "file path")
	flag.Parse()
	var msg proto.Message
	switch *t {
	case "action_result":
		msg = &repb.ActionResult{}
	case "action":
		msg = &repb.Action{}
	case "command":
		msg = &repb.Command{}
	case "dir":
		msg = &repb.Directory{}
	case "all":
		msg = &repb.Action{}
		ReadProto(*file, msg)
		PrintProto(msg)
		command := msg.(*repb.Action).GetCommandDigest()
		rootDir := (*file)[:strings.LastIndex(*file, "/")]
		rootDir = (rootDir)[:strings.LastIndex(rootDir, "/")]
		commandPath := filepath.Join(rootDir, command.GetHash()[:caches.HashPrefixDirPrefixLen], command.GetHash())
		Iter("command", commandPath)
		dir := msg.(*repb.Action).GetInputRootDigest()
		dirPath := filepath.Join(rootDir, dir.GetHash()[:caches.HashPrefixDirPrefixLen], dir.GetHash())
		Iter("dir", dirPath)
		return
	default:
		panic("unknown type " + *t)
	}

	ReadProto(*file, msg)
	PrintProto(msg)
}

func ReadProto(path string, msg proto.Message) {
	content, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	err = proto.Unmarshal(content, msg)
	if err != nil {
		panic(err)
	}
}

func PrintProto(msg proto.Message) {
	fmt.Printf("%T: %s\n", msg, protojson.Format(msg))
}

func Iter(t, file string) {
	a, err := os.Executable()
	if err != nil {
		panic(err)
	}
	cmd := exec.Command(a, "-t", t, "-f", file)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		panic(err)
	}
}
