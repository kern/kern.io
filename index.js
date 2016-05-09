var express = require('express')
var favicon = require('serve-favicon')
var morgan = require('morgan')

var app = express()

if (process.env.NODE_ENV !== 'production') {
  var stylus = require('stylus')
  var nib = require('nib')
  app.get('/app.css', stylus.middleware({
    src: __dirname + '/css',
    dest: __dirname + '/static',
    compile: function (str, path) {
      return stylus(str)
      .set('filename', path)
      .use(nib())
      .import('nib')
    }
  }))
}

app.use(morgan('dev'))
app.use(favicon(__dirname + '/static/favicon.ico'))
app.use(express.static(__dirname + '/static'))

var port = process.env.PORT ? parseInt(process.env.PORT, 10): 3000
var server = app.listen(port, function () {
  var host = server.address().address
  var port = server.address().port
  console.log('kern.io listening on ' + host + ':' + port)
})
