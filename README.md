# Go Count Me

Go Count Me is a KMin Values database with a leveldb backend.  What this allows
you to do is store a massive amount of very large sets and fetch values for
the following operations with relatively low error:

* Cardinality
* Intersection
* Union 
* Jaccard Index

## HTTP Interface

An HTTP server gets spun up if the `gocountme` binary is run.  The server has
the following endpoints:

/get : `key` parameter designating which set to return

/delete : `key` parameter designating which set to delete

/cardinality : `key` parameter designating which set to calculate the
cardinality of

/add : `key` and `hash` parameters saying which set to add the given hash to.
Hashes are signed 64bit integers

/query : `q` which is a url encoded json specifying the desired query (more
about queries below)


## Queries

In order to do efficient lookups of complex set operations, we support a
limited query language that is based on recursively defined json objects.  The
basic json object looks like,

``` 
{ "method" : "...", "keys" : ["...", "...", ], "set" : [ {...}, {...} ] }
``` 

Here, `keys` and `set` are mutually exclusive representations of data and
`method` is what operation to perform on them.  The `keys` list is a list of
direct keys into the database while the `set` list is a set of similarly
defined dictionaries.  As a result, we can calculate a complex quantity such as:

```
Jaccard( key1 u key2, key8 n key3 )
```

with the following query,

```
{
    "method" : "jaccard",
    "set" : [
        {
            "method" : "union",
            "keys" : ["key1", "key2"],
        },
        {
            "method" : "intersection",
            "keys" : ["key8", "key3"],
        },

    ]
}
```

or the operation:

```
Card( (key1 u key2 u key3) n key5)
```

with the query,

```
{
    "method" : "cardinality",
    "set" : [
        {
            "method" : "intersection",
            "set" : [
                {
                    "method" : "union",
                    "keys" : ["key1", "key2", "key3"]
                },
                {
                    "method" : "get",
                    "keys" : ["key5"]
                },
            ]
        },
    ]
}
```

If a key doesn't exist, then it is treated as an empty set.
