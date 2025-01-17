// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

//go:generate go run github.com/tinylib/msgp -o=activity_dump_graph_gen_linux.go -tests=false

package probe

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/security/probe/dump"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

var (
	processColor         = "#8fbbff"
	processRuntimeColor  = "#edf3ff"
	processSnapshotColor = "white"
	processShape         = "record"

	fileColor         = "#77bf77"
	fileRuntimeColor  = "#e9f3e7"
	fileSnapshotColor = "white"
	fileShape         = "record"

	networkColor        = "#ff9800"
	networkRuntimeColor = "#ffebcd"
	networkShape        = "record"
)

// NodeGenerationType is used to indicate if a node was generated by a runtime or snapshot event
type NodeGenerationType string

var (
	// Runtime is a node that was added at runtime
	Runtime NodeGenerationType = "runtime"
	// Snapshot is a node that was added during the snapshot
	Snapshot NodeGenerationType = "snapshot"
)

type node struct {
	ID        GraphID
	Label     string
	Size      int
	Color     string
	FillColor string
	Shape     string
	IsTable   bool
}

type edge struct {
	Link  string
	Color string
}

type graph struct {
	Title string
	Nodes map[GraphID]node
	Edges []edge
}

// GraphTemplate is the template used to generate graphs
var GraphTemplate = `digraph {
		label = "{{ .Title }}"
		labelloc =  "t"
		fontsize = 75
		fontcolor = "black"
		fontname = "arial"
		ratio = expand
		ranksep = 2

		graph [pad=2]
		node [margin=0.3, padding=1, penwidth=3]
		edge [penwidth=2]

		{{ range .Nodes }}
		{{ .ID }} [label={{ if not .IsTable }}"{{ end }}{{ .Label }}{{ if not .IsTable }}"{{ end }}, fontsize={{ .Size }}, shape={{ .Shape }}, fontname = "arial", color="{{ .Color }}", fillcolor="{{ .FillColor }}", style="filled"]{{ end }}

		{{ range .Edges }}
		{{ .Link }} [arrowhead=none, color="{{ .Color }}"]
		{{ end }}
}`

// EncodeDOT encodes an activity dump in the DOT format
func (ad *ActivityDump) EncodeDOT() (*bytes.Buffer, error) {
	ad.Lock()
	defer ad.Unlock()

	title := fmt.Sprintf("%s: %s", ad.DumpMetadata.Name, ad.getSelectorStr())
	data := ad.prepareGraphData(title)
	t := template.Must(template.New("tmpl").Parse(GraphTemplate))
	raw := bytes.NewBuffer(nil)
	if err := t.Execute(raw, data); err != nil {
		return nil, fmt.Errorf("couldn't encode %s in %s: %w", ad.getSelectorStr(), dump.DOT, err)
	}
	return raw, nil
}

func (ad *ActivityDump) prepareGraphData(title string) graph {
	data := graph{
		Title: title,
		Nodes: make(map[GraphID]node),
	}

	for _, p := range ad.ProcessActivityTree {
		ad.prepareProcessActivityNode(p, &data)
	}

	return data
}

func (ad *ActivityDump) prepareProcessActivityNode(p *ProcessActivityNode, data *graph) {
	var args string
	if ad.adm != nil && ad.adm.probe != nil {
		if argv, _ := ad.adm.probe.resolvers.ProcessResolver.GetProcessScrubbedArgv(&p.Process); len(argv) > 0 {
			args = strings.ReplaceAll(strings.Join(argv, " "), "\"", "\\\"")
			args = strings.ReplaceAll(args, "\n", " ")
			args = strings.ReplaceAll(args, ">", "\\>")
			args = strings.ReplaceAll(args, "|", "\\|")
		}
	}
	pan := node{
		ID:    NewGraphID(p.GetID()),
		Label: fmt.Sprintf("%s %s", p.Process.FileEvent.PathnameStr, args),
		Size:  60,
		Color: processColor,
		Shape: processShape,
	}
	switch p.GenerationType {
	case Runtime:
		pan.FillColor = processRuntimeColor
	case Snapshot:
		pan.FillColor = processSnapshotColor
	}
	data.Nodes[NewGraphID(p.GetID())] = pan

	for _, n := range p.Sockets {
		ad.prepareSocketNode(n, data, p.GetID())
	}
	for _, n := range p.DNSNames {
		data.Edges = append(data.Edges, edge{
			Link:  fmt.Sprintf("%s -> %s", p.GetID(), NewGraphID(p.GetID(), n.GetID())),
			Color: networkColor,
		})
		ad.prepareDNSNode(n, data, p.GetID())
	}
	for _, f := range p.Files {
		data.Edges = append(data.Edges, edge{
			Link:  fmt.Sprintf("%s -> %s", p.GetID(), NewGraphID(p.GetID(), f.GetID())),
			Color: fileColor,
		})
		ad.prepareFileNode(f, data, "", p.GetID())
	}
	if len(p.Syscalls) > 0 {
		ad.prepareSyscallsNode(p, data)
	}
	for _, child := range p.Children {
		data.Edges = append(data.Edges, edge{
			Link:  fmt.Sprintf("%s -> %s", p.GetID(), child.GetID()),
			Color: processColor,
		})
		ad.prepareProcessActivityNode(child, data)
	}
}

func (ad *ActivityDump) prepareDNSNode(n *DNSNode, data *graph, processID NodeID) {
	if len(n.Requests) == 0 {
		// save guard, this should never happen
		return
	}
	name := n.Requests[0].Name + " (" + (model.QType(n.Requests[0].Type).String())
	for _, req := range n.Requests[1:] {
		name += ", " + model.QType(req.Type).String()
	}
	name += ")"

	dnsNode := node{
		ID:        NewGraphID(processID, n.GetID()),
		Label:     name,
		Size:      30,
		Color:     networkColor,
		FillColor: networkRuntimeColor,
		Shape:     networkShape,
	}
	data.Nodes[dnsNode.ID] = dnsNode
}

func (ad *ActivityDump) prepareSocketNode(n *SocketNode, data *graph, processID NodeID) {
	targetID := NewGraphID(processID, n.GetID())

	// prepare main socket node
	data.Edges = append(data.Edges, edge{
		Link:  fmt.Sprintf("%s -> %s", processID, targetID),
		Color: networkColor,
	})
	data.Nodes[targetID] = node{
		ID:        targetID,
		Label:     n.Family,
		Size:      30,
		Color:     networkColor,
		FillColor: networkRuntimeColor,
		Shape:     networkShape,
	}

	// prepare bind nodes
	var names []string
	for _, node := range n.Bind {
		names = append(names, fmt.Sprintf("[%s]:%d", node.IP, node.Port))
	}

	for i, name := range names {
		socketNode := node{
			ID:        NewGraphID(processID, n.GetID(), NodeID(i+1)),
			Label:     name,
			Size:      30,
			Color:     networkColor,
			FillColor: networkRuntimeColor,
			Shape:     networkShape,
		}
		data.Edges = append(data.Edges, edge{
			Link:  fmt.Sprintf("%s -> %s", NewGraphID(processID, n.GetID()), socketNode.ID),
			Color: networkColor,
		})
		data.Nodes[socketNode.ID] = socketNode
	}
}

func (ad *ActivityDump) prepareFileNode(f *FileActivityNode, data *graph, prefix string, processID NodeID) {
	mergedID := NewGraphID(processID, f.GetID())
	fn := node{
		ID:    mergedID,
		Label: f.getNodeLabel(),
		Size:  30,
		Color: fileColor,
		Shape: fileShape,
	}
	switch f.GenerationType {
	case Runtime:
		fn.FillColor = fileRuntimeColor
	case Snapshot:
		fn.FillColor = fileSnapshotColor
	}
	data.Nodes[mergedID] = fn

	for _, child := range f.Children {
		data.Edges = append(data.Edges, edge{
			Link:  fmt.Sprintf("%s -> %s", mergedID, NewGraphID(processID, child.GetID())),
			Color: fileColor,
		})
		ad.prepareFileNode(child, data, prefix+f.Name, processID)
	}
}

func (ad *ActivityDump) prepareSyscallsNode(p *ProcessActivityNode, data *graph) {
	label := fmt.Sprintf("<<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"1\"> <TR><TD><b>arch: %s</b></TD></TR>", ad.Arch)
	for _, s := range p.Syscalls {
		label += "<TR><TD>" + model.Syscall(s).String() + "</TD></TR>"
	}
	label += "</TABLE>>"

	syscallsNode := node{
		ID:        NewGraphID(p.GetID()),
		Label:     label,
		Size:      30,
		Color:     processColor,
		FillColor: processSnapshotColor,
		Shape:     processShape,
		IsTable:   true,
	}
	data.Nodes[syscallsNode.ID] = syscallsNode
	data.Edges = append(data.Edges, edge{
		Link:  fmt.Sprintf("%s -> %s", p.GetID(), syscallsNode.ID),
		Color: processColor,
	})
}

// GraphID represents an ID used in a graph, combination of NodeIDs
//msgp:ignore GraphID
type GraphID struct {
	raw string
}

// NewGraphID returns a new GraphID based on the provided NodeIDs
func NewGraphID(ids ...NodeID) GraphID {
	return NewGraphIDWithDescription("", ids...)
}

// NewGraphIDWithDescription returns a new GraphID based on a description and on the provided NodeIDs
func NewGraphIDWithDescription(description string, ids ...NodeID) GraphID {
	var b strings.Builder
	if description != "" {
		b.WriteString(description)
	} else {
		b.WriteString("node")
	}

	for _, id := range ids {
		b.WriteString("_")
		b.WriteString(id.String())
	}

	return GraphID{
		raw: b.String(),
	}
}
