package ai

// Status 节点执行状态
type Status int

const (
	StatusSuccess Status = iota
	StatusFailure
	StatusRunning
)

// Node 行为树节点接口
type Node interface {
	Tick(bb *Blackboard) Status
}

// Sequence 顺序节点：遇到 Failure 停止，全 Success 才 Success
type Sequence struct {
	Children []Node
}

func (s *Sequence) Tick(bb *Blackboard) Status {
	for _, child := range s.Children {
		status := child.Tick(bb)
		if status != StatusSuccess {
			return status
		}
	}
	return StatusSuccess
}

// Selector 选择节点：遇到 Success 停止，全 Failure 才 Failure
type Selector struct {
	Children []Node
}

func (s *Selector) Tick(bb *Blackboard) Status {
	for _, child := range s.Children {
		status := child.Tick(bb)
		if status != StatusFailure {
			return status
		}
	}
	return StatusFailure
}

// Action 动作节点
type Action struct {
	Do func(bb *Blackboard) Status
}

func (a *Action) Tick(bb *Blackboard) Status {
	return a.Do(bb)
}

// Condition 条件节点
type Condition struct {
	Check func(bb *Blackboard) bool
}

func (c *Condition) Tick(bb *Blackboard) Status {
	if c.Check(bb) {
		return StatusSuccess
	}
	return StatusFailure
}
