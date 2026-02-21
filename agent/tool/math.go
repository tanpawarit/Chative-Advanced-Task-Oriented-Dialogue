package tool

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
)

const (
	ToolMathEvaluate = "math.evaluate"
)

// Accepts digits, whitespace, decimal points, operators, and parentheses.
var mathExpressionPattern = regexp.MustCompile(`^[\d\s\+\-\*/%\^\(\)\.]+$`)

type MathEvaluateOutput struct {
	Expression string  `json:"expression"`
	Result     float64 `json:"result"`
}

func executeMathTool(tool string, args map[string]any) (contractx.ToolResult, error) {
	rawExpression, ok := args["expression"]
	if !ok {
		return contractx.ToolResult{
			Tool:  tool,
			Error: "expression is required",
		}, nil
	}

	expression, ok := rawExpression.(string)
	if !ok {
		return contractx.ToolResult{
			Tool:  tool,
			Error: "expression must be a string",
		}, nil
	}

	expression = strings.TrimSpace(expression)
	if err := validateMathExpression(expression); err != nil {
		return contractx.ToolResult{
			Tool:  tool,
			Error: err.Error(),
		}, nil
	}

	result, err := evaluateMathExpression(expression)
	if err != nil {
		return contractx.ToolResult{
			Tool:  tool,
			Error: err.Error(),
		}, nil
	}

	return contractx.ToolResult{
		Tool: tool,
		Result: MathEvaluateOutput{
			Expression: expression,
			Result:     result,
		},
	}, nil
}

func validateMathExpression(expression string) error {
	if expression == "" {
		return fmt.Errorf("expression is empty")
	}
	if !mathExpressionPattern.MatchString(expression) {
		return fmt.Errorf("expression contains invalid characters")
	}

	balance := 0
	for _, ch := range expression {
		switch ch {
		case '(':
			balance++
		case ')':
			balance--
			if balance < 0 {
				return fmt.Errorf("expression has unbalanced parentheses")
			}
		}
	}
	if balance != 0 {
		return fmt.Errorf("expression has unbalanced parentheses")
	}
	return nil
}

func evaluateMathExpression(expression string) (float64, error) {
	p := &mathParser{input: expression}
	value, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if p.hasNext() {
		return 0, fmt.Errorf("unexpected token at position %d", p.pos)
	}
	return value, nil
}

type mathParser struct {
	input string
	pos   int
}

func (p *mathParser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}

	for {
		p.skipSpaces()
		switch {
		case p.match('+'):
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			left += right
		case p.match('-'):
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			left -= right
		default:
			return left, nil
		}
	}
}

func (p *mathParser) parseTerm() (float64, error) {
	left, err := p.parsePower()
	if err != nil {
		return 0, err
	}

	for {
		p.skipSpaces()
		switch {
		case p.match('*'):
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			left *= right
		case p.match('/'):
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
		case p.match('%'):
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			left = math.Mod(left, right)
		default:
			return left, nil
		}
	}
}

func (p *mathParser) parsePower() (float64, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}

	p.skipSpaces()
	if p.match('^') {
		right, err := p.parsePower()
		if err != nil {
			return 0, err
		}
		return math.Pow(left, right), nil
	}
	return left, nil
}

func (p *mathParser) parseUnary() (float64, error) {
	p.skipSpaces()
	if p.match('+') {
		return p.parseUnary()
	}
	if p.match('-') {
		value, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return -value, nil
	}
	return p.parsePrimary()
}

func (p *mathParser) parsePrimary() (float64, error) {
	p.skipSpaces()
	if p.match('(') {
		value, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpaces()
		if !p.match(')') {
			return 0, fmt.Errorf("missing closing parenthesis at position %d", p.pos)
		}
		return value, nil
	}
	return p.parseNumber()
}

func (p *mathParser) parseNumber() (float64, error) {
	p.skipSpaces()
	start := p.pos
	hasDigit := false
	hasDot := false

	for p.hasNext() {
		ch := p.peek()
		switch {
		case ch >= '0' && ch <= '9':
			hasDigit = true
			p.pos++
		case ch == '.':
			if hasDot {
				return 0, fmt.Errorf("invalid number format at position %d", p.pos)
			}
			hasDot = true
			p.pos++
		default:
			goto done
		}
	}

done:
	if !hasDigit {
		return 0, fmt.Errorf("expected number at position %d", start)
	}

	raw := p.input[start:p.pos]
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q: %w", raw, err)
	}
	return value, nil
}

func (p *mathParser) skipSpaces() {
	for p.hasNext() && p.peek() == ' ' {
		p.pos++
	}
}

func (p *mathParser) hasNext() bool {
	return p.pos < len(p.input)
}

func (p *mathParser) peek() byte {
	return p.input[p.pos]
}

func (p *mathParser) match(expected byte) bool {
	if p.hasNext() && p.peek() == expected {
		p.pos++
		return true
	}
	return false
}
