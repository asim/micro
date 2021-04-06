// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Original source: github.com/micro/go-micro/v3/util/registry/util.go

package registry

import (
	"github.com/micro/micro/v3/service/registry"
)

func addNodes(old, neu []*registry.Node) []*registry.Node {
	nodes := make([]*registry.Node, len(neu))
	// add all new nodes
	for i, n := range neu {
		node := *n
		nodes[i] = &node
	}

	// look at old nodes
	for _, o := range old {
		var exists bool

		// check against new nodes
		for _, n := range nodes {
			// ids match then skip
			if o.Id == n.Id {
				exists = true
				break
			}
		}

		// keep old node
		if !exists {
			node := *o
			nodes = append(nodes, &node)
		}
	}

	return nodes
}

func delNodes(old, del []*registry.Node) []*registry.Node {
	var nodes []*registry.Node
	for _, o := range old {
		var rem bool
		for _, n := range del {
			if o.Id == n.Id {
				rem = true
				break
			}
		}
		if !rem {
			nodes = append(nodes, o)
		}
	}
	return nodes
}

// CopyService make a copy of service
func CopyService(service *registry.Service) *registry.Service {
	// copy service
	s := new(registry.Service)
	*s = *service

	// copy nodes
	nodes := make([]*registry.Node, len(service.Nodes))
	for j, node := range service.Nodes {
		n := new(registry.Node)
		*n = *node
		nodes[j] = n
	}
	s.Nodes = nodes

	// copy endpoints
	eps := make([]*registry.Endpoint, len(service.Endpoints))
	for j, ep := range service.Endpoints {
		e := new(registry.Endpoint)
		*e = *ep
		eps[j] = e
	}
	s.Endpoints = eps
	return s
}

// Copy makes a copy of services
func Copy(current []*registry.Service) []*registry.Service {
	services := make([]*registry.Service, len(current))
	for i, service := range current {
		services[i] = CopyService(service)
	}
	return services
}

// Merge merges two lists of services and returns a new copy
func Merge(olist, nlist []*registry.Service) []*registry.Service {
	services := Copy(nlist)

	for _, n := range olist {
		var seen bool
		for _, o := range services {
			if o.Version == n.Version {
				seen = true
				break
			}
		}

		if !seen {
			//services = append(services, Copy([]*registry.Service{n})...)
			services = append(services, CopyService(n))
		}
	}

	return services
}

// Remove removes services and returns a new copy
func Remove(old, del []*registry.Service) []*registry.Service {
	var services []*registry.Service

	for _, o := range old {
		srv := new(registry.Service)
		*srv = *o

		var rem bool

		for _, s := range del {
			if srv.Version == s.Version {
				srv.Nodes = delNodes(srv.Nodes, s.Nodes)

				if len(srv.Nodes) == 0 {
					rem = true
				}
			}
		}

		if !rem {
			services = append(services, srv)
		}
	}

	return services
}
