package hdl

import (
	"fmt"
	"strings"

	"dbt-rules/RULES/core"
	"dbt-rules/hdl"
)

type XSimScriptParams struct {
	Name         string
	PartName     string
	BoardName    string
	OutDir       core.Path
	OutScript    core.Path
	OutSimScript core.Path
	IncDir       core.Path
	Srcs         []core.Path
	Ips          []core.Path
	Libs         []string
	Verbose      bool
}

type SimulationXsim struct {
	Name    string
	Srcs    []core.Path
	Ips     []Ip
	Libs    []string
	Verbose bool
}

func (rule SimulationXsim) Build(ctx core.Context) {
	outDir := ctx.Cwd().WithSuffix("/" + rule.Name)
	outScript := outDir.WithSuffix(".sh")
	outSimScript := outDir.WithSuffix(".xsim.tcl")

	ins := []core.Path{}
	srcs := []core.Path{}
	ips := []core.Path{}

	for _, ip := range FlattenIpGraph(rule.Ips) {
		for _, src := range ip.Sources() {
			if IsSimulationArchive(src.String()) {
				ips = append(ips, src)
			} else if IsRtl(src.String()) {
				srcs = append(srcs, src)
			}
			ins = append(ins, src)
		}
	}
	srcs = append(srcs, rule.Srcs...)
	ins = append(ins, rule.Srcs...)

	data := XSimScriptParams{
		PartName:     PartName.Value(),
		BoardName:    BoardName.Value(),
		Name:         strings.ToLower(rule.Name),
		OutDir:       outDir,
		OutScript:    outScript,
		OutSimScript: outSimScript,
		IncDir:       core.SourcePath(""),
		Srcs:         srcs,
		Ips:          ips,
		Libs:         rule.Libs,
		Verbose:      rule.Verbose,
	}

	ctx.AddBuildStep(core.BuildStep{
		Outs:   []core.OutPath{outDir, outScript, outSimScript},
		Ins:    ins,
		Script: core.CompileTemplateFile(hdl.XSimScriptTmpl.String(), data),
		Descr:  fmt.Sprintf("Generating XSim simulation %s", outScript.Relative()),
	})
}
