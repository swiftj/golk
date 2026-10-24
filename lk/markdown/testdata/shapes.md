###### paragraph

Hello

###### paragraph empty

::paragraph

###### paragraph empty with attrs

::paragraph align=center background=#ffffff

###### paragraph align left

a
{align=left}

###### paragraph background empty string

a
{background=""}

###### paragraph unknown align

```lk-raw
{
  "kind": "paragraph",
  "align": "justify",
  "content": [
    {
      "text": "a"
    }
  ]
}
```

###### paragraph unmodelled field

```lk-raw
{
  "kind": "paragraph",
  "content": [
    {
      "text": "a"
    }
  ],
  "legacy": 1
}
```

###### run unmodelled field

```lk-raw
{
  "kind": "paragraph",
  "content": [
    {
      "text": "a",
      "glow": true
    }
  ]
}
```

###### raw run

```lk-raw
{
  "kind": "paragraph",
  "content": [
    {
      "emoji": "x"
    }
  ]
}
```

###### empty text run

```lk-raw
{
  "kind": "paragraph",
  "content": [
    {
      "text": ""
    }
  ]
}
```

###### adjacent equal runs

```lk-raw
{
  "kind": "paragraph",
  "content": [
    {
      "text": "a"
    },
    {
      "text": "b"
    }
  ]
}
```

###### newline in text

```lk-raw
{
  "kind": "paragraph",
  "content": [
    {
      "text": "a\nb"
    }
  ]
}
```

###### trailing break

```lk-raw
{
  "kind": "paragraph",
  "content": [
    {
      "text": "a"
    },
    {
      "break": true
    }
  ]
}
```

###### leading break

\
a

###### double break

a\
\
b

###### marks

**b**[ i]{italic}~~s~~`c`[u]{underline}[2]{sub}[3]{sup}[col]{color=#EC4899}[hl]{highlight=#F59E0B}

###### all marks one run

[***~~[`z`]{underline sub sup color=#000000 highlight=#111111}~~***](https://x.test)

###### bold with edge spaces

Hello[ world ]{bold}!

###### nested marks

[a]{bold}[b]{bold italic}[c]{italic}

###### bold punctuation inside word

a["q"]{bold}b

###### link

see [here](https://x.test/a_\(b\)?c=1\&d=2)

###### link with spaces and entity

[x](<https://x.test/a b\&amp;c\<d\>>)

###### empty link

[x](<>)

###### lk link

[x](lk://resource/abc)

###### two links

[a](u1)[b](u2)

###### code with backticks

```a `` b```

###### code edge spaces

`  x  `

###### code one space

` `

###### code starting backtick

`` `a ``

###### text needing escapes

\# not \*a\* \_heading\_ \[x\](y) \{z\} \<b> \\ \~s\~ \`c\` \| \&amp; \&#35; !

###### line start markers

\- a\
1\. b\
\> c\
\+ d\
\===\
\:::secret\
&#32;&#32;&#32;&#32;code?\
\---

###### edge whitespace

&#9;&#32;two  spaces&#32;&#32;

###### only spaces

&#32;&#32;&#32;

###### bang before span

wow\![x]{underline}

###### bang before link

wow\![x](u)

###### unicode

Château — ★ 日本

###### control character

```lk-raw
{
  "kind": "paragraph",
  "content": [
    {
      "text": "a\u0001b"
    }
  ]
}
```

###### mention

[Princess Bubblegum]{mention=pb1}

###### mention no name

[]{mention=pb1}

###### mention empty name

[]{mention=pb1 name=""}

###### mention label

[PB]{mention=pb1 label}

###### mention empty label

[]{mention=pb1 label}

###### mention label name doc colors

[PB \[x\]]{mention="pb 1" doc=d1 label name=Bubble*gum color=#ff0000 highlight=#00ff00}

###### mention unmodelled

```lk-raw
{
  "kind": "paragraph",
  "content": [
    {
      "mention": {
        "resourceId": "pb1",
        "extra": 1
      }
    }
  ]
}
```

###### mentions adjacent

[]{mention=a}[]{mention=b}

###### heading

## Title

###### heading all levels 6

###### T \#1 \#

###### heading empty

#

###### heading empty attrs

### {align=center}

###### heading attrs

# [x ]{bold}y\} {align=right background=#3B82F6}

###### heading level 7

```lk-raw
{
  "kind": "heading",
  "level": 7,
  "content": [
    {
      "text": "x"
    }
  ]
}
```

###### heading level 0

```lk-raw
{
  "kind": "heading",
  "level": 0,
  "content": [
    {
      "text": "x"
    }
  ]
}
```

###### heading break

```lk-raw
{
  "kind": "heading",
  "level": 1,
  "content": [
    {
      "text": "a"
    },
    {
      "break": true
    },
    {
      "text": "b"
    }
  ]
}
```

###### quote

> x
>
> y

###### quote color

> x
{color=#ff0000}

###### quote empty

>

###### quote nested

> > x
> {color=#111111}
{color=#000000}

###### quote holding everything

> ## h
>
> ```
> a
> ::: b
> > b
> ```
>
> - x
>
> :::callout tone=note
> x
> :::
>
> ---

###### code

```go
func main() {}

```

###### code no language

```
x
```

###### code empty

```

```

###### code newline only

```


```

###### code with fences

`````md
```
````
:::callout

`````

###### code with a closing fence line

```lk-raw
{
  "kind": "code",
  "text": "a\n:::\n\u003e b"
}
```

###### code with a secret fence line

`````lk-raw
{
  "kind": "code",
  "language": "md",
  "text": "```\n````\n:::secret\n"
}
`````

###### code with tabs

```
	indented
  two
```

###### code language spaces

```c sharp
x
```

###### code language empty

```lk-raw
{
  "kind": "code",
  "language": "",
  "text": "x"
}
```

###### code language lk-raw

```lk-raw
{
  "kind": "code",
  "language": "lk-raw",
  "text": "{}"
}
```

###### code CR

```lk-raw
{
  "kind": "code",
  "text": "a\rb"
}
```

###### rule

---

###### bullet list

- x
- y

###### bullet empty

```lk-raw
{
  "kind": "bulletList",
  "children": []
}
```

###### bullet item empty

-
- x

###### bullet item empty paragraph

- ::paragraph

###### bullet item two paragraphs

- x

  y

###### bullet item code first

- ```
  x
  ```

###### bullet nested lists

- x
  - y

  * x

  3. x

###### bullet with break

- a\
  \- b

###### bullet non-item child

```lk-raw
{
  "kind": "bulletList",
  "children": [
    {
      "kind": "paragraph",
      "content": [
        {
          "text": "x"
        }
      ]
    }
  ]
}
```

###### ordered

1. x

###### ordered start 1

```lk-raw
{
  "kind": "orderedList",
  "start": 1,
  "children": [
    {
      "kind": "listItem",
      "children": [
        {
          "kind": "paragraph",
          "content": [
            {
              "text": "x"
            }
          ]
        }
      ]
    }
  ]
}
```

###### ordered start 0

0. x

###### ordered start 9

9. x
10. y

###### ordered negative

```lk-raw
{
  "kind": "orderedList",
  "start": -1,
  "children": [
    {
      "kind": "listItem",
      "children": [
        {
          "kind": "paragraph",
          "content": [
            {
              "text": "x"
            }
          ]
        }
      ]
    }
  ]
}
```

###### task list

- [ ] a
- [x] **\[x\] b**

###### task empty item

- [ ]

###### task item leading space

- [x] &#32;a

###### task nested

- [ ] a
  - [x] b
  * [x] c
    - [x] d
- [ ] e\
  f

###### task nested first

```lk-raw
{
  "kind": "taskList",
  "children": [
    {
      "kind": "taskList",
      "children": [
        {
          "kind": "taskItem",
          "checked": false,
          "content": [
            {
              "text": "a"
            }
          ]
        }
      ]
    }
  ]
}
```

###### task empty

```lk-raw
{
  "kind": "taskList",
  "children": []
}
```

###### table gfm

| **H\|1** |  |
| --- | --- |
| `a\|b` | [ x ](u\|v) |

###### table header only

| x |
| --- |

###### table spans

:::table
:::row
:::cell header colspan=2 rowspan=1
x
:::
:::
:::row
:::cell header=false
x

y
:::
:::cell
:::
:::
:::

###### table no header

:::table
:::row
:::cell
x
:::
:::
:::

###### table ragged

:::table
:::row
:::cell header
x
:::
:::cell header
x
:::
:::
:::row
:::cell
x
:::
:::
:::

###### table empty

:::table
:::

###### table cell attrs paragraph

:::table
:::row
:::cell header
x
{align=center}
:::
:::
:::

###### table cell break

:::table
:::row
:::cell header
a\
b
:::
:::
:::

###### table row non-cell

```lk-raw
{
  "kind": "table",
  "children": [
    {
      "kind": "tableRow",
      "children": [
        {
          "kind": "paragraph",
          "content": [
            {
              "text": "x"
            }
          ]
        }
      ]
    }
  ]
}
```

###### callout

:::callout tone=warning
x
:::

###### callout custom

:::callout tone=custom icon=dragon color=#aa0000
x

:::callout tone=info
:::
:::

###### callout unknown tone

```lk-raw
{
  "kind": "callout",
  "tone": "loud",
  "children": []
}
```

###### callout empty tone

```lk-raw
{
  "kind": "callout",
  "tone": "",
  "children": []
}
```

###### expand

:::expand title="Hidden \"details\" {x}"
x
:::

###### expand empty title

:::expand title=""
:::

###### expand newline title

```lk-raw
{
  "kind": "expand",
  "title": "a\nb",
  "children": []
}
```

###### layout

:::layout
:::column width=50 background=#3B82F6
x
:::
:::column width=33.333333333333336
:::
:::column width=1e+21
y
:::
:::

###### layout non-column

```lk-raw
{
  "kind": "layout",
  "children": [
    {
      "kind": "paragraph",
      "content": [
        {
          "text": "x"
        }
      ]
    }
  ]
}
```

###### secret

:::secret layout=wrapLeft width=30
x
:::

###### secret empty

```lk-raw
{
  "kind": "secret",
  "children": []
}
```

###### secret nested

:::secret
:::secret
```
:::
```
:::

x
:::

###### textfield

:::textfield title=Origin isTitleHidden=false layout=center width=100
x
:::

###### textfield empty

:::textfield
:::

###### image

![](https://assets.legendkeeper.com/p.png)

###### image all fields

![A \[map\]](<https://assets.legendkeeper.com/p (1).png>){fill=false title="" isTitleHidden layout=wrapRight width=40}

###### image empty alt

![](u){alt=""}

###### image empty url

![](<>)

###### embed

::embed provider=youtube url=https://www.youtube.com/watch?v=x title=T isTitleHidden=false layout=default width=0

###### embed unknown provider

```lk-raw
{
  "kind": "embed",
  "provider": "vimeo",
  "url": "u"
}
```

###### keyvalue

:::keyvalue display=grid title=Stats isTitleHidden layout=alignEnd width=10
- Species :: Magic dog
- Plays :: **Video games** {text}
- a \:\: b :: c :: d {text=other}
-  ::
- Text only {text=t}
- Nothing
- &#32;&#32;pad&#32;&#32; :: \{x\}
:::

###### keyvalue empty

:::keyvalue
:::

###### keyvalue row unmodelled

```lk-raw
{
  "kind": "keyValue",
  "rows": [
    {
      "key": "a",
      "flag": 1
    }
  ]
}
```

###### keyvalue empty key no content

```lk-raw
{
  "kind": "keyValue",
  "rows": [
    {
      "key": ""
    }
  ]
}
```

###### meter

:::meter mode=gauge-180 showText showIcon=false showMax title=Vitals
- HP :: 3/5 icon=heart color=#ff0000
- MP \:\: x :: 0.5/-1e-07 icon="" ratingIcon=star
:::

###### meter empty

:::meter mode=bar
:::

###### meter unknown mode

```lk-raw
{
  "kind": "meter",
  "mode": "pie",
  "items": []
}
```

###### links

:::links title="See also"
- @finn1
- @"jake 1" doc=d9
:::

###### links empty

:::links
:::

###### collection items

:::collection title=Residents isTitleHidden
- @finn1
:::

###### collection filter

:::collection
```lk-filter
{"type":"filter","id":"f1","filterType":"tag","operator":"has_any","tags":["Kingdom"],"future":{"x":[1,2]}}
```
:::

###### collection items and filter

:::collection
- @a

````lk-filter
{"type":"filter","id":"f1","filterType":"composite","operator":"and","filters":[{"type":"filter","id":"g","filterType":"text","value":"x```"}]}
````
:::

###### collection empty items

```lk-raw
{
  "kind": "resourceCollection",
  "items": []
}
```

###### collection nothing

:::collection
:::

###### map

::map resourceId=atlas documentId=atlasMap viewport=12.5,-3,2 displayHeight=400.5 fill title=World isTitleHidden=false layout=wrapLeft width=50

###### map empty

::map

###### map odd viewport

```lk-raw
{
  "kind": "map",
  "viewport": {
    "center": [
      1
    ],
    "zoom": 2
  }
}
```

###### infobox

:::infobox layout=wrapRight width=40
![Portrait](https://assets.legendkeeper.com/p.png){title=Portrait isTitleHidden isHidden=false fill}

:::keyvalue title=Stats display=inline
- Species :: Magic dog
:::

:::textfield title=Origin
x
:::

:::meter mode=rating
- Fame :: 2/5 icon=star
:::

:::links isHidden
- @a
:::

:::collection
```lk-filter
{"type":"filter","id":"f","filterType":"tag"}
```
:::

::embed provider=spotify url=https://open.spotify.com/x

::map resourceId=a documentId=b viewport=0,0,1

::unsupported id=u2 title=Weird

```lk-raw
{
  "kind": "hologram",
  "id": "h2",
  "beam": true
}
```

```lk-raw
{
  "kind": "textField",
  "children": [],
  "extra": 1
}
```
:::

###### infobox empty

:::infobox
:::

###### unsupported

::unsupported id=u1

###### unsupported quoted id

::unsupported id="a b"

###### unsupported unmodelled

```lk-raw
{
  "kind": "unsupported",
  "id": "u3",
  "x": 1
}
```

###### unknown kind

````lk-raw
{
  "kind": "hologram",
  "id": "h1",
  "nested": {
    "a": [
      1,
      "```"
    ]
  }
}
````

###### known kind bad shape

```lk-raw
{
  "kind": "paragraph",
  "content": 5
}
```

