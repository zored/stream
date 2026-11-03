package main

type layer struct {
	name   string
	active func() bool
	press  func() error
	up     func() error
}

type slot struct {
	vars   *vars
	name   string
	layers []layer
	held   *layer
}

func (s *slot) top() layer {
	for _, l := range s.layers[:len(s.layers)-1] {
		if l.active() {
			return l
		}
	}
	return s.layers[len(s.layers)-1]
}

func (s *slot) publish() {
	s.vars.set(s.name, s.top().name)
}

func (s *slot) press() error {
	l := s.top()
	s.held = &l
	s.vars.set(s.name, l.name)
	return l.press()
}

func (s *slot) up() error {
	l := s.held
	s.held = nil
	if l == nil || l.up == nil {
		return nil
	}
	return l.up()
}
