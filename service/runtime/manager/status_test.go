package manager

import (
	"testing"

	"github.com/micro/go-micro/v3/runtime"
	"github.com/micro/micro/v3/internal/namespace"
	"github.com/micro/micro/v3/profile"
	muruntime "github.com/micro/micro/v3/service/runtime"
)

type testRuntime struct {
	createCount  int
	readCount    int
	updateCount  int
	deleteCount  int
	readServices []*runtime.Service
	events       chan *runtime.Service
	runtime.Runtime
}

func (r *testRuntime) String() string {
	return "test"
}

func (r *testRuntime) Reset() {
	r.createCount = 0
	r.readCount = 0
	r.updateCount = 0
	r.deleteCount = 0
}

func (r *testRuntime) Create(srv *runtime.Service, opts ...runtime.CreateOption) error {
	r.createCount++
	if r.events != nil {
		r.events <- srv
	}
	return nil
}
func (r *testRuntime) Update(srv *runtime.Service, opts ...runtime.UpdateOption) error {
	r.updateCount++
	if r.events != nil {
		r.events <- srv
	}
	return nil
}
func (r *testRuntime) Delete(srv *runtime.Service, opts ...runtime.DeleteOption) error {
	r.deleteCount++
	if r.events != nil {
		r.events <- srv
	}
	return nil
}

func (r *testRuntime) Read(opts ...runtime.ReadOption) ([]*runtime.Service, error) {
	r.readCount++
	return r.readServices, nil
}

func TestStatus(t *testing.T) {
	testServices := []*runtime.Service{
		&runtime.Service{
			Name:     "foo",
			Version:  "latest",
			Status:   runtime.Starting,
			Metadata: map[string]string{},
		},
		&runtime.Service{
			Name:     "bar",
			Version:  "2.0.0",
			Status:   runtime.Error,
			Metadata: map[string]string{"error": "Crashed on L1"},
		},
	}

	profile.Test.Setup(nil)
	muruntime.DefaultRuntime = &testRuntime{readServices: testServices}
	m := New().(*manager)

	// sync the status with the runtime, this should set the status for the testServices in the cache
	m.syncStatus()

	// get the statuses from the service
	statuses, err := m.listStatuses(namespace.DefaultNamespace)
	if err != nil {
		t.Fatalf("Unexpected error when listing statuses: %v", err)
	}

	// loop through the test services and check the status matches what was set in the metadata
	for _, srv := range testServices {
		s, ok := statuses[srv.Name+":"+srv.Version]
		if !ok {
			t.Errorf("Missing status for %v:%v", srv.Name, srv.Version)
			continue
		}
		if s.Status != srv.Status {
			t.Errorf("Incorrect status for %v:%v, expepcted %v but got %v", srv.Name, srv.Version, srv.Status, s.Status)
		}
		if s.Error != srv.Metadata["error"] {
			t.Errorf("Incorrect error for %v:%v, expepcted %v but got %v", srv.Name, srv.Version, srv.Metadata["error"], s.Error)
		}
	}

	// update the status for a service and check it correctly updated
	srv := testServices[0]
	srv.Status = runtime.Running
	if err := m.cacheStatus(namespace.DefaultNamespace, srv); err != nil {
		t.Fatalf("Unexpected error when caching status: %v", err)
	}

	// get the statuses from the service
	statuses, err = m.listStatuses(namespace.DefaultNamespace)
	if err != nil {
		t.Fatalf("Unexpected error when listing statuses: %v", err)
	}

	// check the new status matches the changed service
	s, ok := statuses[srv.Name+":"+srv.Version]
	if !ok {
		t.Errorf("Missing status for %v:%v", srv.Name, srv.Version)
	}
	if s.Status != srv.Status {
		t.Errorf("Incorrect status for %v:%v, expepcted %v but got %v", srv.Name, srv.Version, srv.Status, s.Status)
	}
	if s.Error != srv.Metadata["error"] {
		t.Errorf("Incorrect error for %v:%v, expepcted %v but got %v", srv.Name, srv.Version, srv.Metadata["error"], s.Error)
	}
}
