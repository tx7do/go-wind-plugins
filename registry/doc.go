// Package registry defines the service-discovery abstractions for the
// go-wind framework.
//
// It splits service registration and discovery into two independent
// interfaces ([Registrar] and [Discovery]) so that callers can mix providers
// (etcd, consul, zookeeper, etc.) as needed.
//
//   - [Registrar] — used by service providers to register/deregister themselves.
//   - [Discovery] — used by service consumers to discover live instances.
package registry
