package interfaces

import "reflect"

type ServiceLifetime int

const (
	Singleton ServiceLifetime = iota
	Transient
	Scoped
)

type Container interface {
	Register(serviceType reflect.Type, factory interface{}, lifetime ServiceLifetime) error
	RegisterSingleton(serviceType reflect.Type, factory interface{}) error
	RegisterTransient(serviceType reflect.Type, factory interface{}) error
	RegisterInstance(serviceType reflect.Type, instance interface{}) error
	Resolve(serviceType reflect.Type) (interface{}, error)
	ResolveAll(serviceType reflect.Type) ([]interface{}, error)
	CreateScope() Container
	Build() error
}

type ServiceFactory func(c Container) (interface{}, error)