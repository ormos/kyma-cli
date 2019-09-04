## kyma completion

Generates bash completion scripts

### Synopsis

Output shell completion code for bash. The shell code must be evaluated to provide
interactive completion of commands. This can be done by sourcing it from the .bash _profile.
To load completion, run:

. <(kyma completion)

To configure your bash shell to load completions for each session, add to your bashrc:

# ~/.bashrc or ~/.profile
. <(kyma completion)


```
kyma completion [flags]
```

### Options

```
  -h, --help   help for completion
```

### Options inherited from parent commands

```
      --kubeconfig string   Path to kubeconfig (default "/Users/i504462/.kube/config")
      --non-interactive     Do not use spinners
  -v, --verbose             verbose output
```

### SEE ALSO

* [kyma](kyma.md)	 - Controls a Kyma cluster.

###### Auto generated by spf13/cobra on 3-Sep-2019