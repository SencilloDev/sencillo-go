## sgoctl new service

Creates a new service

### Synopsis

Creates a new Sencillo microservice from a template

```
sgoctl new service [flags]
```

### Options

```
      --container-registry string   URL for container registry (default "example.com")
      --disable-deployment          Disables Kubernetes deployment generation
      --domain string               Domain for ingress URLs (default "example.com")
      --enable-edgedb               Enable EdgeDB integration
      --enable-graphql              Enables GraphQL integration
      --enable-http                 Enables HTTP integration
      --enable-telemetry            Enable opentelemetry integration
  -h, --help                        help for service
      --metrics-url string          Endpoint for metrics exporter (default "localhost:4318")
  -n, --name string                 Application name
      --namespace string            Namespace for deployment (default "default")
      --nats-service string         NATS server urls
```

### Options inherited from parent commands

```
      --config string   config file (default is $HOME/.sgo.yaml)
  -d, --debug           Print output instead of creating files
```

### SEE ALSO

* [sgoctl new](sgoctl_new.md)	 - Creates a new Sencillo app

###### Auto generated by spf13/cobra on 7-Jan-2025
