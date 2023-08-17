package world_logger

import (
	"fmt"
	"github.com/google/uuid"
	"log"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"reflect"
	"runtime"
	"strings"
)

type WorldLogger struct {
	logger *log.Logger
	world  *ecs.World
}

func componentToString(component component.IComponentType) string {
	return fmt.Sprintf("component_id: %d, component_name: %s", component.ID(), component.Name())
}

func (wl *WorldLogger) componentsLogString(id string) string {
	var buffer strings.Builder
	buffer.WriteString(fmt.Sprintf("log_id: %s, total_components: %d\n", id, len(wl.world.ListRegisteredComponents())))
	for _, _component := range wl.world.ListRegisteredComponents() {
		buffer.WriteString(fmt.Sprintf("	log_id: %s, %s\n", id, componentToString(_component)))
	}
	return buffer.String()
}

func systemToString(system *ecs.System) string {
	functionName := runtime.FuncForPC(reflect.ValueOf(system).Pointer()).Name()

	return fmt.Sprintf("system_name: %s", functionName)
}

func (wl *WorldLogger) systemLogString(id string) string {
	var buffer strings.Builder
	buffer.WriteString(fmt.Sprintf("log_id: %s, total_systems: %d\n", id, len(wl.world.ListSystems())))
	for _, system := range wl.world.ListSystems() {
		buffer.WriteString(fmt.Sprintf("	log_id: %s, %s\n", id, systemToString(&system)))
	}
	return buffer.String()
}

func (wl *WorldLogger) LogWorldState() {
	id := uuid.New().String()
	var buffer strings.Builder
	buffer.WriteString(wl.componentsLogString(id))
	buffer.WriteString(wl.systemLogString(id))
	wl.logger.Print(buffer.String())
}
