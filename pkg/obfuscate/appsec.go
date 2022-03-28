// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"fmt"
	"sort"
	"strings"
)

// unexpectedScannerOpError is an error wrapper type around a scanner operation
// that was unexpectedly encountered a scanning function. Returning such an
// error allows the caller to adapt its parsing to what was actually encountered
// and proceed accordingly, for example by actually returning successfully when
// the end of an array was encountered instead of an array value.
type unexpectedScannerOpError int

func (err unexpectedScannerOpError) Error() string {
	return fmt.Sprintf("unexpected json scanner operation %d", err)
}

// diff specifies a set of changes consisting of byte offsets and a string to
// replace them in an original input.
type diff []struct {
	from, to int
	value    string
}

// apply the diff to the input if any.
func (d diff) apply(input string) string {
	from := 0
	var output strings.Builder
	for _, diff := range d {
		output.WriteString(input[from:diff.from])
		from = diff.to + 1
		output.WriteString(diff.value)
	}
	output.WriteString(input[from:])
	return output.String()
}

// add a new modification to the diff, starting at from and ending at to, to
// be replaced with value.
func (d *diff) add(from, to int, value string) {
	elt := struct {
		from  int
		to    int
		value string
	}{
		from:  from,
		to:    to,
		value: value,
	}
	diff := *d
	// We want the list sorted incrementally: use binary search to find the
	// index for the new entry.
	i := sort.Search(len(diff), func(i int) bool {
		return diff[i].to > from
	})
	if i == len(diff) {
		*d = append(diff, elt)
		return
	}
	*d = append(diff[:i+1], diff[i:]...)
	(*d)[i] = elt
}

// merge the given diff by shifting its from/to offsets by adding the given
// offset to them.
func (d *diff) merge(diff diff, offset int) {
	for _, diff := range diff {
		d.add(diff.from+offset, diff.to+offset, diff.value)
	}
}

// ObfuscateAppSec obfuscates the given appsec tag value in order to remove
// sensitive values from the security events.
func (o *Obfuscator) ObfuscateAppSec(val string) string {
	keyRE := o.opts.AppSec.KeyRegexp
	valueRE := o.opts.AppSec.ValueRegexp
	if keyRE == nil && valueRE == nil {
		return val
	}
	output, err := o.obfuscateAppSec(val)
	if err != nil {
		o.log.Errorf("Unexpected error while obfuscating the appsec events: %v", err)
		return val
	}
	return output
}

// Entrypoint of the appsec obfuscator based on a json scanner and which tries
// to only obfuscate the appsec rule parameters of the given input by searching
// and targeting objects of the form
// `{ "parameters": <appsec rule match parameters>, <any other keys>: <any other values> }`
// When such object is found, its parameters' value is obfuscated.
func (o *Obfuscator) obfuscateAppSec(input string) (output string, err error) {
	var (
		scanner scanner
		diff    diff
	)
	scanner.reset()
	keyFrom := -1 // marks the starting index of a JSON key in bytes
	keyTo := -1   // marks the end index of a JSON key in bytes
	// Scan the input to discover its objects and see if they have the
	// `parameters` key we are looking for.
	for i := 0; i < len(input); i++ {
		switch scanner.step(&scanner, input[i]) {
		case scanError:
			return input, scanner.err

		case scanBeginLiteral:
			// Possibly the beginning of an object key
			if input[i] == '"' {
				keyFrom = i
			}
		case scanContinue:
			// Continue the literal value
			if keyFrom != -1 && isJSONStringEnd(input, i) {
				// Ending double-quote found
				keyTo = i
				// Only scanObjetKey will confirm this was an object key
			}
		case scanObjectKey:
			// Object key scanned and keyFrom and keyTo were set according to
			// the previous scanBeginLiteral and scanContinue scanner operations
			if input[keyFrom:keyTo+1] == `"parameters"` {
				i = o.obfuscateAppSecParameters(&scanner, input, i+1, &diff)
				i-- // decrement i due to the for loop increment but i is already set to the next byte to scan
			}
			keyFrom = -1
			keyTo = -1
		}
	}
	// Tell the scanner the end of the input has been reach
	if scanner.eof() == scanError {
		return input, scanner.err
	}
	return diff.apply(input), nil
}

// Obfuscate the array of parameters of the form `[ <parameter 1>, <...>, <parameter N> ]`.
// The implementation accepts elements of unexpected types.
func (o *Obfuscator) obfuscateAppSecParameters(scanner *scanner, input string, i int, diff *diff) int {
	i, err := stepTo(scanner, input, i, scanBeginArray)
	if err != nil {
		return i
	}
	for ; i < len(input); i++ {
		i, err = o.obfuscateAppSecParameter(scanner, input, i, diff)
		if err != nil {
			got, ok := err.(unexpectedScannerOpError)
			if !ok {
				return i // Abort due to an unexpected error (syntax error or end of json)
			}
			switch got {
			case scanEndArray:
				// The previous call failed because we reached the end of the array
				// This case happens for the empty array value
				return i
			case scanBeginObject:
				// Ignore the nested object
				i, err = stepUntil(scanner, input, i, scanEndObject)
			case scanBeginArray:
				// Ignore the nested array
				i, err = stepUntil(scanner, input, i, scanEndArray)
			case scanBeginLiteral:
				// Let the following stepToOneOf do the job and scan until the next array value
				err = nil
			}
			if err != nil {
				return i
			}
		}
		// Step to the beginning of the next array value or end of the array
		var op int
		i, op, err = stepToOneOf(scanner, input, i, scanArrayValue, scanEndArray)
		if err != nil || op == scanEndArray {
			return i
		}
	}
	return i
}

// Obfuscate the parameter object of the form `{ "key_path": <key path>, "highlight": <highlight>, "value": <value>, <any extra keys>: <any extra values> }`.
// The implementation is permissive so that any extra keys and values are allowed, and permits having any of the three
// keys `key_path`, `highlight` and `value` that we need to obfuscate.
// Note that the overall parameter obfuscation directly depends on the presence of a sensitive key in the key_path.
// As a result, the parameter object needs to be entirely walked to firstly find the key_path.
func (o *Obfuscator) obfuscateAppSecParameter(scanner *scanner, input string, i int, d *diff) (int, error) {
	// Walk the object and save the `key_path` value along with the offsets of the
	// `highlight` and `value` values, if any.
	var (
		keypath    string // keypath is the value of the "key_path" field, if any
		vfrom, vto int    // vfrom & vto are the start and end offsets of the "value" field in the input, in any
		hfrom, hto int    // hfrom & hto are the start and end offsets of the "highlight" field in the input, in any
	)
	i, err := walkObject(scanner, input, i, func(keyFrom, keyTo int, valueFrom, valueTo int) {
		switch input[keyFrom:keyTo] {
		case `"key_path"`:
			keypath = input[valueFrom:valueTo]
		case `"value"`:
			vfrom = valueFrom
			vto = valueTo
		case `"highlight"`:
			hfrom = valueFrom
			hto = valueTo
		}
	})
	if err != nil {
		return i, err
	}
	// Firstly start by searching for any sensitive key into the key_path
	hasSensitiveKey := o.hasSensitiveKeyPath(keypath)
	// Finally, obfuscate the `highlight` and `value` values
	if highlights := input[hfrom:hto]; highlights != "" {
		var tmpDiff diff
		o.obfuscateAppSecHighlights(highlights, &tmpDiff, hasSensitiveKey)
		d.merge(tmpDiff, hfrom)
	}
	if value := input[vfrom:vto]; value != "" {
		var tmpDiff diff
		o.obfuscateAppSecValue(value, &tmpDiff, hasSensitiveKey)
		d.merge(tmpDiff, vfrom)
	}
	return i, nil
}

// Return whether the given key path contains any sensitive key. A key is
// considered sensitive when the key regular expression matches it. It only
// applies to key path elements of string type.
// The expected key path value is of the form `[ <path 1>, <...>, <path N> ]`.
// The implementation is permissive so that any array value type is accepted.
func (o *Obfuscator) hasSensitiveKeyPath(keypath string) bool {
	// Shortcut the call if the key regular expression is disabled
	keyRE := o.opts.AppSec.KeyRegexp
	if keyRE == nil || keypath == "" {
		return false
	}
	// Walk the array values of type string
	var found bool
	walkArrayStrings(keypath, func(from, to int) {
		// Ignore the call if we already found a sensitive key in a previous call
		if found {
			return
		}
		// Unquote the string and check if it matches the key regexp
		value := keypath[from : to+1]
		value, ok := unquote(value)
		if !ok {
			return
		}
		if keyRE.MatchString(value) {
			found = true
		}
	})
	return found
}

// Obfuscate the parameter's array of highlighted strings of the form `[ <highlight 1>, <...>, <highlight N> ]`.
// If a sensitive key was found, the value regular expression is ignored and every string value of the array is
// obfuscated. It otherwise only obfuscates the sub-strings matching the value regular expression.
// The implementation is permissive so that it accepts any value type and only obfuscates the strings.
// Note that this obfuscator method is a bit different from the others due to the way obfuscateAppSecParameter()
// works.
func (o *Obfuscator) obfuscateAppSecHighlights(input string, diff *diff, hasSensitiveKey bool) {
	// Shortcut the call when the value regular expression is disabled and there
	// is no sensitive key (which acts as a regexp obfuscating everything)
	valueRE := o.opts.AppSec.ValueRegexp
	if valueRE == nil && !hasSensitiveKey {
		return
	}
	walkArrayStrings(input, func(from, to int) {
		if hasSensitiveKey {
			diff.add(from, to, `"?"`)
			return
		}
		value, ok := unquote(input[from : to+1])
		if !ok {
			return
		}
		if !valueRE.MatchString(value) {
			return
		}
		value = valueRE.ReplaceAllString(value, "?")
		value, err := quote(value)
		if err != nil {
			return
		}
		diff.add(from, to, value)
	})
}

// Obfuscate the parameter's value which is expected to be a string. If a
// sensitive key was found, the value regular expression is ignored and the
// entire string value is obfuscated. It otherwise only obfuscates the
// sub-strings matching the value regular expression.
// Note that this obfuscator method is a bit different from the others due to
// the way obfuscateAppSecParameter() works.
func (o *Obfuscator) obfuscateAppSecValue(input string, diff *diff, hasSensitiveKey bool) {
	// Shortcut the call when the value regular expression is disabled and there
	// is no sensitive key (which acts as a regexp obfuscating everything)
	valueRE := o.opts.AppSec.ValueRegexp
	if valueRE == nil && !hasSensitiveKey {
		return
	}
	from, to, err := scanString(input)
	if err != nil {
		return
	}
	if hasSensitiveKey {
		diff.add(from, to-1, `"?"`)
		return
	}
	value := input[from:to]
	value, ok := unquote(value)
	if !ok {
		return
	}
	if !valueRE.MatchString(value) {
		return
	}
	value = valueRE.ReplaceAllString(value, "?")
	value, err = quote(value)
	if err != nil {
		return
	}
	diff.add(from, to-1, value)
}

// Helper function to walk the array elements of type string.
func walkArrayStrings(input string, visit func(from int, to int)) {
	var scanner scanner
	scanner.reset()
	i, err := stepTo(&scanner, input, 0, scanBeginArray)
	if err != nil {
		return
	}
	stringFrom := -1
	depth := 0
	for ; i < len(input); i++ {
		c := input[i]
		switch scanner.step(&scanner, c) {
		case scanBeginObject, scanBeginArray:
			depth++
		case scanEndObject:
			depth--
		case scanEndArray:
			if depth == 0 {
				return
			}
			depth--

		case scanBeginLiteral:
			if depth == 0 && input[i] == '"' {
				stringFrom = i
			}
		case scanContinue:
			if stringFrom != -1 && isJSONStringEnd(input, i) {
				visit(stringFrom, i)
				stringFrom = -1
			}

		case scanError:
			return
		}
	}
}

// Helper function to walk the object keys and values.
func walkObject(scanner *scanner, input string, i int, visit func(keyFrom, keyTo, valueFrom, valueTo int)) (int, error) {
	i, err := stepTo(scanner, input, i, scanBeginObject)
	if err != nil {
		return i, err
	}
	keyFrom := -1   // marks the starting index of a JSON object key in bytes
	keyTo := -1     // marks the end index of a JSON object key in bytes
	valueFrom := -1 // marks the starting index of a JSON object value in bytes
	depth := 0
	for ; i < len(input); i++ {
		switch scanner.step(scanner, input[i]) {
		case scanBeginObject, scanBeginArray:
			depth++
		case scanEndArray:
			depth--
		case scanEndObject:
			if depth != 0 {
				depth--
				continue
			}
			// We reached the end of the object we were scanning
			if keyFrom != -1 && keyTo != -1 && valueFrom != -1 {
				// Visit the last value of the objet
				visit(keyFrom, keyTo, valueFrom, i)
			}
			return i + 1, nil
		case scanBeginLiteral:
			// Maybe the beginning of an object key
			if depth != 0 || keyFrom != -1 {
				// We are in a nested object value or not parsing an object key
				continue
			}
			if input[i] == '"' {
				// Save the offset of the beginning of the object key
				keyFrom = i
			}
		case scanContinue:
			// Save the offset of the end of the object key if we are parsing one (keyFrom != -1 && keyTo == -1)
			// and the current character is the ending double-quote
			if keyFrom != -1 && keyTo == -1 && isJSONStringEnd(input, i) {
				keyTo = i + 1
			}
		case scanObjectKey:
			if depth == 0 {
				// Save the beginning of the object value
				valueFrom = i + 1
			}
		case scanObjectValue:
			if depth != 0 {
				continue
			}
			// Visit the object entry
			visit(keyFrom, keyTo, valueFrom, i)
			keyFrom = -1
			keyTo = -1
			valueFrom = -1
		case scanError:
			return i, scanner.err
		}
	}
	scanner.eof() // tell the scanner the end of the input has been reached
	return i, scanner.err
}

// stepTo steps the scanner with `input` starting at `i` until it finds the
// operation `to`. It returns its index when it succeeds or an error when the
// scanner gets an error or an error in case of a syntax error.
func stepTo(scanner *scanner, input string, i int, to int) (int, error) {
	i, _, err := stepToOneOf(scanner, input, i, to)
	return i, err
}

// stepToOneOf is the variadic equivalent of stepTo() by taking the list of
// operations to step to, and returning the operation it found when successfully.
func stepToOneOf(scanner *scanner, input string, i int, to ...int) (j int, op int, err error) {
	for ; i < len(input); i++ {
		switch op := scanner.step(scanner, input[i]); op {
		case scanSkipSpace, scanContinue:
			continue
		case scanError:
			return i + 1, op, scanner.err
		default:
			for _, to := range to {
				if to == op {
					return i + 1, op, nil
				}
			}
			return i + 1, op, unexpectedScannerOpError(op)
		}
	}
	scanner.eof() // tell the scanner the end of the input has been reached
	return i, 0, scanner.err
}

// stepUntil steps the scanner with `input` starting at `i` until it finds the
// operation `until` at the current level of depth. Any nested arrays or objects
// are ignored. It returns its index when it succeeds or an error in case of a
// syntax error.
func stepUntil(scanner *scanner, input string, i int, until int) (int, error) {
	depth := 0
	for ; i < len(input); i++ {
		switch op := scanner.step(scanner, input[i]); op {
		case scanError:
			return i + 1, scanner.err
		case scanSkipSpace, scanContinue:
			continue
		default:
			if depth == 0 && op == until {
				return i + 1, nil
			}
			switch op {
			case scanBeginArray, scanBeginObject:
				depth++
			case scanEndArray, scanEndObject:
				depth--
			}
		}
	}
	scanner.eof() // tell the scanner the end of the input has been reached
	return i, scanner.err
}

// Helper function to scan the string value of the given input. It returns the
// from/to offsets of the input so that the json string is located at
// `input[from:to]`.
func scanString(input string) (from, to int, err error) {
	var scanner scanner
	scanner.reset()
	i, err := stepTo(&scanner, input, 0, scanBeginLiteral)
	if err != nil {
		return 0, 0, err
	}
	from = i - 1
	// Check this is a string literal by checking if the character starting the
	// literal is a double-quote
	if input[from] != '"' {
		return from, i + 1, unexpectedScannerOpError(scanBeginLiteral)
	}
	// Scan the input until we find the double-quote the string
	for ; i < len(input); i++ {
		switch op := scanner.step(&scanner, input[i]); op {
		case scanError:
			return from, i + 1, scanner.err
		case scanContinue:
			// Check if the current character is a double-quote ending the
			// string by checking that it is not escaped by the previous one.
			if isJSONStringEnd(input, i) {
				to := i + 1
				return from, to, nil
			}
		default:
			return from, i + 1, unexpectedScannerOpError(op)
		}
	}
	// We reached the end of the input without finding the last string
	// double-quote and therefore results into a json syntax error
	scanner.eof() // tell the scanner that the end of the input has been reached
	return 0, 0, scanner.err
}

// Return true when the given input at offset i is the end of a JSON string.
func isJSONStringEnd(input string, i int) bool {
	// A JSON string should end with a double-quote that is not escaped.
	return input[i] == '"' && input[i-1] != '\\'
}
