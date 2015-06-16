var express = require('express')
var nib = require('nib')
var stylus = require('stylus')

var app = express()

app.get('/app.css', stylus.middleware({
  src: __dirname + '/css',
  dest: __dirname + '/static',
  compile: function (str, path) {
    return stylus(str)
    .set('filename', path)
    .set('compress', true)
    .use(nib())
    .import('nib')
  }
}))

app.use(express.static(__dirname + '/static'))

var server = app.listen(process.env.PORT || 3000, function () {
  var host = server.address().address
  var port = server.address().port
  console.log('kern.io listening on ' + host + ':' + port)
})
