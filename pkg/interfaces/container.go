package interfaces

import "reflect"

// ServiceLifetime defines how service instances are managed
type ServiceLifetime int

const (
	Singleton ServiceLifetime = iota
	Transient
	Scoped
)

// Container defines dependency injection container interface
type Container interface {
	Register(serviceType reflect.Type, factory interface{}, lifetime ServiceLifetime) error
	RegisterInstance(serviceType reflect.Type, instance interface{}) error
	Resolve(serviceType reflect.Type) (interface{}, error)
	ResolveAll(serviceType reflect.Type) ([]interface{}, error)
	Build() error
}