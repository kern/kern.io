Frank.server.handler = 'thin'
Frank.server.hostname = '0.0.0.0'
Frank.server.port = '3000'

Frank.static_folder = 'static'
Frank.dynamic_folder = 'dynamic'
Frank.layouts_folder = 'layouts'

Frank.sass_options = {
  :style => :compressed,
  :load_paths => ['.', './dynamic/css']
}