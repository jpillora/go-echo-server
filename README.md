# go-echo-server

View your requests in JSON format ([Demo](https://echo.jpillora.com/foo/bar))

```
$ curl https://echo.jpillora.com/foo/bar
```

``` json
{
  "Time": "2015-08-04T07:38:42.621571632Z",
  "Duration": "471.825772ms",
  "Location": "AU-SYD",
  "IP": "1.2.3.4",
  "Proto": "http",
  "Host": "echo.jpillora.com",
  "Method": "GET",
  "Path": "/foo/bar",
  "Headers": {
    "accept": "*/*",
    "accept-encoding": "gzip",
    "connect-time": "0",
    "connection": "close",
    "total-route-time": "0",
    "user-agent": "curl/7.37.1",
    "via": "1.1 vegur"
  }
}
```

### Install

**Binaries**

See [the latest release](https://github.com/jpillora/go-echo-server/releases/latest)

**Source**

``` sh
$ go get -v github.com/jpillora/go-echo-server
```

#### MIT License

Copyright © 2014 Jaime Pillora &lt;dev@jpillora.com&gt;

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
'Software'), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED 'AS IS', WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
