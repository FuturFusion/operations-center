# Cluster Template

Cluster templates provide a convenient way to create multiple clusters with
similar configurations by defining a reusable template with variables that can
be customized for each cluster instance.

A cluster template consists of 3 parts:

* [Service Configuration Template](#service-configuration-template)
* [Application Configuration Template](#application-configuration-template)
* [Variable Definitions](#variable-definitions)

The variables used in the cluster templates have the format `@VARIABLE_NAME@`,
where only alphanumeric characters and underscores are allowed.

During creation of a cluster template, Operations Center verifies, that all
variables in the templates are contained in the variable definitions and that
all variables defined in the variable definition are actually used in the
templates.

The placeholders are replaced with the actual value provided "as-is". It is the
administrator's responsibility to ensure that the resulting configuration file
stays valid (e.g. strings in double quotes). As of now, there is no
sophisticated logic (e.g. conditions) possible beside of pure string replacement.

## Service Configuration Template

Service configuration templates have the exact same structure as regular
[service configurations](cluster.md#service-configuration), but they can include
variables that will be replaced with actual values when the template is used.

## Application Configuration Template

Application configuration templates follow the same principle, taking the same
structure as [application configurations](cluster.md#application-configuration)
allowing variables to be defined and substituted during cluster creation.

## Variable Definitions

The variable definition file (YAML) defines each variable as a top-level key.
For each variable a description and optionally a default value can be provided.

For example:

```yaml
SOME_VARIABLE:
  description: Description of the variable
  default: Default value # (optional)
```

## Use of Cluster Templates

During [template based clustering](cluster.md#template-based-clustering), the
administrator may specify a cluster templates together with a variables file
containing the concrete values, that should be used to replace the variables in
the template.

When the cluster template is used, the administrator provides a file containing
key value pairs for the variables. Operations Center then checks, that the
provided values cover all variables without default values.
