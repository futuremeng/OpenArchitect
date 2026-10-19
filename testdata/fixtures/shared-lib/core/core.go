package core

import "example.com/shared/edge"

type Server struct{ Port int }

func (s *Server) Addr() string { return edge.Version() }
