package container

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/laborin/emed/pkg/interfaces"
)

type serviceDefinition struct {
	factory  interface{}
	lifetime interfaces.ServiceLifetime
	instance interface{}
}

type DIContainer struct {
	services   map[reflect.Type]*serviceDefinition
	singletons map[reflect.Type]interface{}
	parent     *DIContainer
	mu         sync.RWMutex
}

func New() *DIContainer {
	return &DIContainer{
		services:   make(map[reflect.Type]*serviceDefinition),
		singletons: make(map[reflect.Type]interface{}),
	}
}

func (c *DIContainer) Register(serviceType reflect.Type, factory interface{}, lifetime interfaces.ServiceLifetime) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if serviceType == nil {
		return fmt.Errorf("service type cannot be nil")
	}
	if factory == nil {
		return fmt.Errorf("factory cannot be nil")
	}

	factoryType := reflect.TypeOf(factory)
	if factoryType.Kind() != reflect.Func {
		return fmt.Errorf("factory must be a function")
	}

	c.services[serviceType] = &serviceDefinition{
		factory:  factory,
		lifetime: lifetime,
	}

	return nil
}

func (c *DIContainer) RegisterSingleton(serviceType reflect.Type, factory interface{}) error {
	return c.Register(serviceType, factory, interfaces.Singleton)
}

func (c *DIContainer) RegisterTransient(serviceType reflect.Type, factory interface{}) error {
	return c.Register(serviceType, factory, interfaces.Transient)
}

func (c *DIContainer) RegisterInstance(serviceType reflect.Type, instance interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if serviceType == nil {
		return fmt.Errorf("service type cannot be nil")
	}
	if instance == nil {
		return fmt.Errorf("instance cannot be nil")
	}

	c.services[serviceType] = &serviceDefinition{
		lifetime: interfaces.Singleton,
		instance: instance,
	}
	c.singletons[serviceType] = instance

	return nil
}

func (c *DIContainer) Resolve(serviceType reflect.Type) (interface{}, error) {
	c.mu.RLock()
	def, exists := c.services[serviceType]
	c.mu.RUnlock()

	if !exists {
		if c.parent != nil {
			return c.parent.Resolve(serviceType)
		}
		return nil, fmt.Errorf("service %v not registered", serviceType)
	}

	switch def.lifetime {
	case interfaces.Singleton:
		return c.resolveSingleton(serviceType, def)
	case interfaces.Transient:
		return c.createInstance(def)
	case interfaces.Scoped:
		if c.parent != nil {
			return c.resolveSingleton(serviceType, def)
		}
		return c.createInstance(def)
	default:
		return nil, fmt.Errorf("unknown service lifetime: %v", def.lifetime)
	}
}

func (c *DIContainer) ResolveAll(serviceType reflect.Type) ([]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []interface{}
	
	for st := range c.services {
		if st == serviceType || st.Implements(serviceType) {
			instance, err := c.Resolve(st)
			if err != nil {
				return nil, err
			}
			results = append(results, instance)
		}
	}

	if c.parent != nil {
		parentResults, err := c.parent.ResolveAll(serviceType)
		if err != nil {
			return nil, err
		}
		results = append(results, parentResults...)
	}

	return results, nil
}

func (c *DIContainer) CreateScope() interfaces.Container {
	return &DIContainer{
		services:   make(map[reflect.Type]*serviceDefinition),
		singletons: make(map[reflect.Type]interface{}),
		parent:     c,
	}
}

func (c *DIContainer) Build() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for serviceType, def := range c.services {
		if def.factory != nil {
			factoryType := reflect.TypeOf(def.factory)
			
			if factoryType.NumOut() < 1 {
				return fmt.Errorf("factory for %v must return at least one value", serviceType)
			}
			
			if factoryType.NumIn() > 0 {
				firstParam := factoryType.In(0)
				containerType := reflect.TypeOf((*interfaces.Container)(nil)).Elem()
				if !firstParam.Implements(containerType) {
					return fmt.Errorf("factory for %v must accept Container as first parameter", serviceType)
				}
			}
		}
	}

	return nil
}

func (c *DIContainer) resolveSingleton(serviceType reflect.Type, def *serviceDefinition) (interface{}, error) {
	c.mu.RLock()
	if instance, exists := c.singletons[serviceType]; exists {
		c.mu.RUnlock()
		return instance, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	if instance, exists := c.singletons[serviceType]; exists {
		c.mu.Unlock()
		return instance, nil
	}

	if def.instance != nil {
		c.singletons[serviceType] = def.instance
		c.mu.Unlock()
		return def.instance, nil
	}
	
	c.mu.Unlock()
	instance, err := c.createInstance(def)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if existing, exists := c.singletons[serviceType]; exists {
		c.mu.Unlock()
		return existing, nil
	}
	c.singletons[serviceType] = instance
	c.mu.Unlock()
	
	return instance, nil
}

func (c *DIContainer) createInstance(def *serviceDefinition) (interface{}, error) {
	if def.factory == nil {
		if def.instance != nil {
			return def.instance, nil
		}
		return nil, fmt.Errorf("no factory or instance provided")
	}

	factoryValue := reflect.ValueOf(def.factory)
	factoryType := factoryValue.Type()

	var args []reflect.Value
	if factoryType.NumIn() > 0 {
		args = append(args, reflect.ValueOf(interfaces.Container(c)))
	}

	results := factoryValue.Call(args)
	
	if len(results) == 0 {
		return nil, fmt.Errorf("factory returned no values")
	}

	if len(results) > 1 && !results[1].IsNil() {
		if err, ok := results[1].Interface().(error); ok {
			return nil, err
		}
	}

	return results[0].Interface(), nil
}