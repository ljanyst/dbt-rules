package core

import (
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

const scriptFileMode = 0755

type Context interface {
	AddBuildStep(BuildStep)
	Cwd() OutPath
	Fatal(format string, a ...interface{})

	addTargetDependency(interface{})
}

// BuildStep represents one build step (i.e., one build command).
// Each BuildStep produces `Out` and `Outs` from `Ins` and `In` by running `Cmd`.
type BuildStep struct {
	Out     OutPath
	Outs    []OutPath
	In      Path
	Ins     []Path
	Depfile OutPath
	Cmd     string
	Script  string
	Descr   string
}

type buildInterface interface {
	Build(ctx Context)
}

type outputsInterface interface {
	Outputs() []Path
}

type descriptionInterface interface {
	Description() string
}

type context struct {
	targetNames        map[interface{}]string
	currentTarget      string
	cwd                OutPath
	targetDependencies []string
	leafOutputs        map[Path]struct{}
	nextRuleID         int
	ninjaFile          strings.Builder
}

func newContext(vars map[string]interface{}) *context {
	ctx := &context{}

	ctx.targetNames = map[interface{}]string{}
	for name := range vars {
		ctx.targetNames[vars[name]] = name
	}

	fmt.Fprintf(&ctx.ninjaFile, "build __phony__: phony\n\n")

	return ctx
}

func (ctx *context) handleTarget(name string, target buildInterface) {
	ctx.currentTarget = name
	ctx.cwd = outPath{path.Dir(name)}
	ctx.leafOutputs = map[Path]struct{}{}
	ctx.targetDependencies = []string{}

	target.Build(ctx)

	ninjaOuts := []string{}
	for out := range ctx.leafOutputs {
		ninjaOuts = append(ninjaOuts, ninjaEscape(out.Absolute()))
	}
	sort.Strings(ninjaOuts)

	printOuts := []string{}
	if iface, ok := target.(outputsInterface); ok {
		for _, out := range iface.Outputs() {
			relPath, _ := filepath.Rel(workingDir(), out.Absolute())
			printOuts = append(printOuts, relPath)
		}
	} else {
		for out := range ctx.leafOutputs {
			relPath, _ := filepath.Rel(workingDir(), out.Absolute())
			printOuts = append(printOuts, relPath)
		}
	}
	sort.Strings(printOuts)

	if len(printOuts) == 0 {
		printOuts = []string{"<no outputs produced>"}
	}

	fmt.Fprintf(&ctx.ninjaFile, "rule r%d\n", ctx.nextRuleID)
	fmt.Fprintf(&ctx.ninjaFile, "  command = echo \"%s\"\n", strings.Join(printOuts, "\\n"))
	fmt.Fprintf(&ctx.ninjaFile, "  description = Created %s:", name)
	fmt.Fprintf(&ctx.ninjaFile, "\n")
	fmt.Fprintf(&ctx.ninjaFile, "build %s: r%d %s %s __phony__\n", name, ctx.nextRuleID, strings.Join(ninjaOuts, " "), strings.Join(ctx.targetDependencies, " "))
	fmt.Fprintf(&ctx.ninjaFile, "\n")
	fmt.Fprintf(&ctx.ninjaFile, "\n")

	ctx.nextRuleID++
}

func (ctx *context) AddBuildStep(step BuildStep) {
	outs := []string{}
	for _, out := range step.Outs {
		outs = append(outs, ninjaEscape(out.Absolute()))
		ctx.leafOutputs[out] = struct{}{}
	}
	if step.Out != nil {
		outs = append(outs, ninjaEscape(step.Out.Absolute()))
		ctx.leafOutputs[step.Out] = struct{}{}
	}
	if len(outs) == 0 {
		return
	}

	ins := []string{}
	for _, in := range step.Ins {
		ins = append(ins, ninjaEscape(in.Absolute()))
		delete(ctx.leafOutputs, in)
	}
	if step.In != nil {
		ins = append(ins, ninjaEscape(step.In.Absolute()))
		delete(ctx.leafOutputs, step.In)
	}

	if step.Script != "" {
		if step.Cmd != "" {
			ctx.Fatal("cannot specify Cmd and Script in a build step")
		}
		script := []byte(step.Script)
		hash := crc32.ChecksumIEEE([]byte(script))
		scriptFileName := fmt.Sprintf("%08X.sh", hash)
		scriptFilePath := path.Join(buildDir(), "..", scriptFileName)
		err := ioutil.WriteFile(scriptFilePath, script, scriptFileMode)
		if err != nil {
			ctx.Fatal("%s", err)
		}
		step.Cmd = scriptFilePath
	}

	fmt.Fprintf(&ctx.ninjaFile, "rule r%d\n", ctx.nextRuleID)
	if step.Depfile != nil {
		depfile := ninjaEscape(step.Depfile.Absolute())
		fmt.Fprintf(&ctx.ninjaFile, "  depfile = %s\n", depfile)
	}
	fmt.Fprintf(&ctx.ninjaFile, "  command = %s\n", step.Cmd)
	if step.Descr != "" {
		fmt.Fprintf(&ctx.ninjaFile, "  description = %s\n", step.Descr)
	}
	fmt.Fprint(&ctx.ninjaFile, "\n")
	fmt.Fprintf(&ctx.ninjaFile, "build %s: r%d %s\n", strings.Join(outs, " "), ctx.nextRuleID, strings.Join(ins, " "))
	fmt.Fprint(&ctx.ninjaFile, "\n\n")

	ctx.nextRuleID++
}

// Cwd returns the build directory of the current target.
func (ctx *context) Cwd() OutPath {
	return ctx.cwd
}

func (ctx *context) Fatal(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "Error while processing target '%s': %s.\n", ctx.currentTarget, msg)
}

func (ctx *context) addTargetDependency(target interface{}) {
	if reflect.TypeOf(target).Kind() != reflect.Ptr {
		fatal("adding target dependency to non-pointer target")
	}
	name, exists := ctx.targetNames[target]
	if !exists {
		fatal("adding target dependency to invalid target")
	}
	ctx.targetDependencies = append(ctx.targetDependencies, name)
}

func ninjaEscape(s string) string {
	return strings.ReplaceAll(s, " ", "$ ")
}
