package bt

type Status int

const (
	StatusSuccess Status = iota
	StatusFailure
	StatusRunning
)

type Node interface {
	Tick(bb Blackboard) Status
}

type Blackboard interface{}

type Selector struct {
	Children []Node
}

func (s *Selector) Tick(bb Blackboard) Status {
	for _, child := range s.Children {
		switch child.Tick(bb) {
		case StatusSuccess:
			return StatusSuccess
		case StatusRunning:
			return StatusRunning
		case StatusFailure:
			continue
		}
	}
	return StatusFailure
}

type Sequence struct {
	Children []Node
}

func (s *Sequence) Tick(bb Blackboard) Status {
	for _, child := range s.Children {
		switch child.Tick(bb) {
		case StatusFailure:
			return StatusFailure
		case StatusRunning:
			return StatusRunning
		case StatusSuccess:
			continue
		}
	}
	return StatusSuccess
}

type ConditionFunc func(bb Blackboard) bool

type Condition struct {
	Check ConditionFunc
}

func (c *Condition) Tick(bb Blackboard) Status {
	if c.Check == nil {
		return StatusFailure
	}
	if c.Check(bb) {
		return StatusSuccess
	}
	return StatusFailure
}

type ActionFunc func(bb Blackboard) Status

type Action struct {
	Do ActionFunc
}

func (a *Action) Tick(bb Blackboard) Status {
	if a.Do == nil {
		return StatusFailure
	}
	return a.Do(bb)
}
