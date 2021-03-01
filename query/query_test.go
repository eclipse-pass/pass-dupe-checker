package query

import (
	"bytes"
	"dupe-checker/model"
	_ "embed"
	"fmt"
	"github.com/knakk/rdf"
	"github.com/stretchr/testify/assert"
	"github.com/yourbasic/graph"
	"github.com/yourbasic/graph/build"
	"html/template"
	"log"
	"strconv"
	"strings"
	"testing"
)

//go:embed queryconfig.json
var queryConfig string

//go:embed queryconfig-simple-or-array.json
var queryConfigSimpleOrArray string

//go:embed queryconfig-simple-or-obj.json
var queryConfigSimpleOrObj string

//go:embed queryconfig-nested-or.json
var queryConfigNestedOr string

//go:embed queryconfig-simple.json
var queryConfigSimple string

//go:embed pass-journal.n3
var passJournal string

//go:embed pass-publication.n3
var passPublication string

func Test_PlanBuilderImplChildrenReturnsTemplates(t *testing.T) {

}

// insures that the Children() method of planBuilderImpl properly recurses child plans, including templates
func Test_PlanBuilderImplChildren(t *testing.T) {
	grandTemplates := []*tmplBuilderImpl{&tmplBuilderImpl{}}
	grandchild := planBuilderImpl{templates: grandTemplates}
	grandchildren := []*planBuilderImpl{&grandchild}

	childTemplates := []*tmplBuilderImpl{&tmplBuilderImpl{}}
	child := planBuilderImpl{templates: childTemplates, children: grandchildren}
	children := []*planBuilderImpl{&child}

	parentTemplates := []*tmplBuilderImpl{&tmplBuilderImpl{}}
	parent := planBuilderImpl{templates: parentTemplates, children: children}

	// parent.Children() should return the immediate child plans and templates.
	assert.Equal(t, 2, len(parent.Children()))
	assert.Equal(t, 2, len(child.Children()))
	assert.Equal(t, 1, len(grandchild.Children()))

	// recursing parent.Children() should return
	//   - parent template
	//   - child plan
	//   - child template
	//   - grandchild plan
	//   - grandchild template
	count := 0
	// note that the recursive verifier invokes the provided function on the supplied plan, so the parent itself is
	// counted.
	recursiveVerifier("", &parent, func(planType string, p Plan) {
		count++
	})
	assert.Equal(t, 6, count, &parent)
}

func recursiveCounter(p Plan, count int) int {
	count += len(p.Children())
	for _, c := range p.Children() {
		return recursiveCounter(c, count)
	}
	return count
}

// verifies the state of the plan
type planVerifier func(planType string, p Plan)

// executes the planVerifier on the given plan and recursively on all its descendents
func recursiveVerifier(planType string, p Plan, pv planVerifier) {
	pv(planType, p)

	for _, child := range p.Children() {
		recursiveVerifier(planType, child, pv)
	}
}

func Test_DecodeSimple(t *testing.T) {
	plans := decoder{}.Decode(queryConfigSimple)

	assert.NotNil(t, plans)
	assert.True(t, len(plans) > 0)
	assert.NotNil(t, plans["http://oapass.org/ns/pass#Journal"])

	expectedTotalPlanCount := 1 // just the root plan, which should be built.
	expectedTotalTemplateCount := 1
	expectedBuiltCount := expectedTotalPlanCount + expectedTotalTemplateCount

	actualBuiltCount, actualTotalPlanCount, actualTotalTemplateCount := 0, 0, 0
	recursiveVerifier("http://oapass.org/ns/pass#Journal", plans["http://oapass.org/ns/pass#Journal"],
		func(planType string, p Plan) {
			assert.Equal(t, "http://oapass.org/ns/pass#Journal", planType)
			switch p.(type) {
			case *tmplBuilderImpl:
				tmplBuilder := p.(*tmplBuilderImpl)
				assert.True(t, tmplBuilder.built)
				assert.EqualValues(t, []string{"nlmta"}, tmplBuilder.keys)
				assert.Equal(t, "es query for nlmta", tmplBuilder.query)
				actualTotalTemplateCount++
			case *planBuilderImpl:
				assert.True(t, p.(*planBuilderImpl).built)
				actualTotalPlanCount++
			}
			actualBuiltCount++
		})

	assert.Equal(t, expectedBuiltCount, actualBuiltCount)
	assert.Equal(t, actualTotalTemplateCount, expectedTotalTemplateCount)
	assert.Equal(t, actualTotalPlanCount, expectedTotalPlanCount)
}

// This JSON is pathological and not allowed
//func Test_DecodeSimpleOrObject(t *testing.T) {
//	plans := decoder{}.Decode(queryConfigSimpleOrObj)
//
//	assert.NotNil(t, plans)
//	assert.True(t, len(plans) > 0)
//
//	verifyPlans(t, plans, 1, 1)
//}

func Test_DecodeSimpleOrArray(t *testing.T) {
	plans := decoder{}.Decode(queryConfigSimpleOrArray)

	assert.NotNil(t, plans)
	assert.Equal(t, 1, len(plans))

	expectedTotalPlanCount := 4 // the root plan and the or plan, and the two queries; all should be built.
	expectedBuiltCount := expectedTotalPlanCount
	verifyPlans(t, plans, expectedBuiltCount, expectedTotalPlanCount)
}

func Test_DecodeNestedOrArray(t *testing.T) {
	plans := decoder{}.Decode(queryConfigNestedOr)

	assert.NotNil(t, plans)
	assert.Equal(t, 1, len(plans))

	expectedTotalPlanCount := 6 // the root plan, two child or plans (one as a child of the other), three templates.
	expectedBuiltCount := expectedTotalPlanCount
	verifyPlans(t, plans, expectedBuiltCount, expectedTotalPlanCount)

}

func verifyPlans(t *testing.T, plans map[string]Plan, expectedBuiltCount, expectedTotalCount int) {
	actualTotalCount := 0
	actualBuiltCount := 0

	for planType, plan := range plans {
		log.Printf("Plan for type %s:\n%s", planType, plan)
		recursiveVerifier(planType, plan, func(planType string, plan Plan) {
			actualTotalCount++
			switch plan.(type) {
			case *tmplBuilderImpl:
				tmplBuilder := plan.(*tmplBuilderImpl)
				assert.True(t, tmplBuilder.built)
				actualBuiltCount++
				assert.NotZero(t, tmplBuilder.query)
				assert.NotZero(t, tmplBuilder.keys)
				for _, v := range tmplBuilder.keys {
					assert.NotZero(t, v)
				}
				//actualTotalTemplateCount++
			case *planBuilderImpl:
				assert.True(t, plan.(*planBuilderImpl).built)
				actualBuiltCount++
				//actualTotalPlanCount++
			}

		})

	}
	assert.Equal(t, expectedBuiltCount, actualBuiltCount)
	assert.Equal(t, expectedTotalCount, actualTotalCount)
}

func Test_Template(t *testing.T) {
	// scheme, host and port, index, type, values
	// http, elasticsearch:9200, pass, type, doi
	// http://elasticsearch:9200/pass?q=@type:<type>+doi:<doi>&default_operator=AND
	fmtStr := "%s://%s/%s?q=@type:%s+doi:%s&default_operator=AND"
	templateStr := "{{.Scheme}}://{{.HostAndPort}}/{{.Index}}?q=@type:{{.Type}}+doi:{{.Doi}}&default_operator=AND"

	tmpl, err := template.New("test").Parse(templateStr)
	assert.Nil(t, err)
	assert.NotNil(t, tmpl)

	buf := &bytes.Buffer{}

	err = tmpl.Execute(buf, struct {
		Scheme      string
		HostAndPort string
		Index       string
		Type        string
		Doi         string
	}{"http", "elasticsearch:9200", "pass", "Submission", "10.1.2.4/567"})

	assert.Nil(t, err)
	assert.Equal(t, fmt.Sprintf(fmtStr, "http", "elasticsearch:9200", "pass", "Submission", "10.1.2.4/567"), buf.String())
}

func Test_TemplateRange(t *testing.T) {
	kvps := []KvPair{{"@type", "Submission"}, {"doi", "10.1.2.4/567"}}

	funcMap := template.FuncMap{
		// The name "inc" is what the function will be called in the template text.
		"inc": func(i int) int {
			return i + 1
		},
		"dec": func(i int) int {
			return i - 1
		},
	}

	// scheme, host and port, index, type, values
	// http, elasticsearch:9200, pass, type, doi
	// http://elasticsearch:9200/pass?q=@type:<type>+doi:<doi>&default_operator=AND
	fmtStr := "%s://%s/%s?q=@type:%s+doi:%s&default_operator=AND"
	//templateStr := "{{.Scheme}}://{{.HostAndPort}}/{{.Index}}?q=@type:{{.Type}}+doi:{{.Doi}}&default_operator=AND"

	templateStr := "{{.Scheme}}://{{.HostAndPort}}/{{.Index}}?q={{$count := dec (len .KvPairs)}}{{range $i, $e := .KvPairs}}{{$e.Key}}:{{$e.Value}}{{if lt $i $count}}+{{end}}{{end}}&default_operator=AND"

	tmpl, err := template.New("test").Funcs(funcMap).Parse(templateStr)
	assert.Nil(t, err)
	assert.NotNil(t, tmpl)

	buf := &bytes.Buffer{}

	err = tmpl.Execute(buf, struct {
		Scheme      string
		HostAndPort string
		Index       string
		KvPairs     []KvPair
	}{"http", "elasticsearch:9200", "pass", kvps})

	assert.Nil(t, err)
	assert.Equal(t, fmt.Sprintf(fmtStr, "http", "elasticsearch:9200", "pass", "Submission", "10.1.2.4/567"), buf.String())

}

func TestTemplate_BuildAndEval(t *testing.T) {

	// normally templateStr and tmplBuilderImpl would be created by parsing the query json config
	// Scheme, HostAndPort, Index must come from env or on construction
	templateStr := "{{.Scheme}}://{{.HostAndPort}}/{{.Index}}?q={{$count := dec (len .KvPairs)}}{{range $i, $e := .KvPairs}}{{$e.Key}}:{{$e.Value}}{{if lt $i $count}}+{{end}}{{end}}&default_operator=AND"
	tmplBuilder := tmplBuilderImpl{
		built: false,
		keys:  []string{"@type", "doi"},
		query: templateStr,
	}

	plan, err := tmplBuilder.Build()

	assert.Nil(t, err)
	assert.IsType(t, Template{}, plan)

	tmpl := plan.(Template)
	esQuery, err := tmpl.eval([]KvPair{{"@type", "Submission"}, {"doi", "10.1.2.4/567"}})

	assert.Nil(t, err)
	assert.Equal(t, "http://elasticsearch:9200/pass?q=@type:Submission+doi:10.1.2.4/567&default_operator=AND", esQuery)
}

func TestTemplate_ExtractKeys(t *testing.T) {
	container, err := model.NewContainerFromReader(strings.NewReader(passJournal), rdf.NTriples)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(container.PassProperties()))

	kvp := extractKeys(container, []string{"journalName", "issn"})
	assert.Equal(t, 3, len(kvp))

	assert.Equal(t, "journalName", kvp[0].Key)
	assert.Equal(t, "Community dentistry and oral epidemiology", kvp[0].Value)
	assert.Equal(t, "issn", kvp[1].Key)
	assert.Equal(t, "Online:1600-0528", kvp[1].Value)
	assert.Equal(t, "issn", kvp[2].Key)
	assert.Equal(t, "Print:0301-5661", kvp[2].Value)
}

func TestTemplate_Execute(t *testing.T) {
	// Normally the container is provided by the repository visitor but here we read it off of the filesystem.
	container, err := model.NewContainerFromReader(strings.NewReader(passPublication), rdf.NTriples)
	assert.Nil(t, err)
	assert.Equal(t, 5, len(container.PassProperties()))

	// normally templateStr and tmplBuilderImpl would be created by parsing the query json config
	// Scheme, HostAndPort, Index must come from env or on construction
	// DOIs need to be quoted!!!
	templateStr := "{{.Scheme}}://{{.HostAndPort}}/{{.Index}}/_search?q={{$count := dec (len .KvPairs)}}{{range $i, $e := .KvPairs}}{{$e.Key}}:\"{{$e.Value}}\"{{if lt $i $count}}+{{end}}{{end}}&default_operator=AND"
	tmplBuilder := tmplBuilderImpl{
		built: false,
		keys:  []string{"@type", "doi"},
		query: templateStr,
	}

	plan, err := tmplBuilder.Build()
	assert.IsType(t, Template{}, plan)

	tmpl := plan.(Template)

	processedResult := false
	err = tmpl.Execute(container, func(result string) error {
		atoi, err := strconv.Atoi(result)
		assert.Nil(t, err)
		assert.Equal(t, 1, atoi)
		processedResult = true
		return nil
	})

	assert.Nil(t, err)
	assert.True(t, processedResult)

}

/*
func Test_DecodeConfig(t *testing.T) {
	plans := make(map[string]Plan)
	var passType string
	var tmplBuilder TemplateBuilder
	builder := newPlanBuilder()
	dec := json.NewDecoder(strings.NewReader(queryConfigSimpleOrObj))
	level := 0
	queryTokenStack := tokenStack{[]token{}}

	for {
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		//log.Printf("handling: %v", t)

		switch t.(type) {
		case json.Delim:
			switch t.(json.Delim).String() {
			case "{":
				level++
			case "}":
				level--
				if queryTokenStack.size() > 0 {
					popped := queryTokenStack.popE()
					switch popped {
					case orT:
						if built, err := tmplBuilder.Build(); err != nil {
							log.Fatal(err)
						} else {
							builder.AddPlan(built)
						}
					case qT:
						// TODO
					case queryT:
						if p, err := builder.Build(); err != nil {
							log.Fatal(err)
						} else {
							plans[passType] = p
						}
					default:
						// TODO
						log.Fatalf("Unhandled popped token %v", popped)
					}
				}
			case "]":
				if queryTokenStack.size() > 0 {
					queryTokenStack.popE()
				}
			}
		case string:
			switch token(t.(string)) {
			case queryT:
				queryTokenStack.pushE(queryT)
			case orT:
				queryTokenStack.pushE(orT)
				// create a new TemplateBuilder, add it to the PlanBuilder, and set the state as the
				// active template being built
				tmplBuilder = builder.Or()
			case andT:
				queryTokenStack.pushE(andT)
				// create a new TemplateBuilder, add it to the PlanBuilder, and set the state as the
				// active template being built
				//tmplBuilder = builder.And()
			case keysT:
				queryTokenStack.pushE(keysT)
			case qT:
				queryTokenStack.pushE(qT)
			default:
				if queryTokenStack.size() > 0 {
					if tmplBuilder == nil {
						log.Fatalf("no template builder present (has PlanBuilder().Or() or PlanBuilder.And() been invoked and stored?)")
					}
					log.Printf("Have a value for '%s': %v", queryTokenStack.peek(), t)
					// add the key or query to the TemplateBuilder
					switch queryTokenStack.peek() {
					case keysT:
						tmplBuilder.AddKey(t.(string))
					case qT:
						tmplBuilder.AddQuery(t.(string))
					default:
						log.Fatalf("Unknown token %v in 'query' object", t)
					}
				} else {
					// have a top level key representing a PASS type
					log.Printf("Have a PASS type: %v", t)
					passType = t.(string)
					//_ = builder.ForResource(t.(string))
				}
			}
		}
		//log.Printf("query stack: %v", queryTokenStack)
	}

	log.Printf("%s", builder)

	/*
		for {
			t, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%T: %v", t, t)
			if dec.More() {
				fmt.Printf(" (more)")
			}
			fmt.Printf("\n")
		}
*/

func Test_Protoype(t *testing.T) {
	g := graph.New(5)
	g.Add(0, 1)
	g.Add(0, 2)
	g.Add(1, 3)
	g.Add(1, 4)

	//g.Visit(0, func(w int, c int64) bool {
	//	log.Printf("Visiting %d", w)
	//	return false
	//})

	visitAll(g)
}

func Test_BuildVirtualGraph(t *testing.T) {
	virtualG := build.Kn(5)

	visitAll(virtualG)
}

func visitAll(g graph.Iterator) {
	for v := 0; v < g.Order(); v++ {
		graph.Sort(g).Visit(v, func(w int, c int64) bool {
			log.Printf("Visiting %d", w)
			return false
		})
	}
}
