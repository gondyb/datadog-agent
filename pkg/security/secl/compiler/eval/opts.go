// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

// Opts are the options to be passed to the evaluator
type Opts struct {
	LegacyFields map[Field]Field
	Constants    map[string]interface{}
	Macros       map[MacroID]*Macro
	Variables    map[string]VariableValue
}

// WithConstants set constants
func (o *Opts) WithConstants(constants map[string]interface{}) *Opts {
	o.Constants = constants
	return o
}

// WithVariables set variables
func (o *Opts) WithVariables(variables map[string]VariableValue) *Opts {
	o.Variables = variables
	return o
}

// WithLegacyFields set legacy fields
func (o *Opts) WithLegacyFields(fields map[Field]Field) *Opts {
	o.LegacyFields = fields
	return o
}
