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

func (wl *WorldLogger) componentsLogString(id string, stringBuilder strings.Builder) strings.Builder {
	stringBuilder.WriteString(fmt.Sprintf("log_id: %s, total_components: %d\n", id, len(wl.world.ListRegisteredComponents())))
	for _, _component := range wl.world.ListRegisteredComponents() {
		stringBuilder.WriteString(fmt.Sprintf("	log_id: %s, %s\n", id, componentToString(_component)))
	}
	return stringBuilder
}

func systemToString(system *ecs.System) string {
	functionName := runtime.FuncForPC(reflect.ValueOf(system).Pointer()).Name()

	return fmt.Sprintf("system_name: %s", functionName)
}

func (wl *WorldLogger) systemLogString(id string, stringBuilder strings.Builder) strings.Builder {
	stringBuilder.WriteString(fmt.Sprintf("log_id: %s, total_systems: %d\n", id, len(wl.world.ListSystems())))
	for _, system := range wl.world.ListSystems() {
		stringBuilder.WriteString(fmt.Sprintf("	log_id: %s, %s\n", id, systemToString(&system)))
	}
	return stringBuilder
}

func (wl *WorldLogger) LogWorldState(id string) {
	if len(id) == 0 {
		id = uuid.New().String()
	}
	var stringBuilder strings.Builder
	stringBuilder = wl.componentsLogString(id, stringBuilder)
	stringBuilder = wl.systemLogString(id, stringBuilder)
	wl.logger.Print(stringBuilder.String())
}
