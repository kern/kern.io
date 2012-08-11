require "redcarpet"
require "pygments"

module Kernul
  class PygmentsMarkdownRenderer < Redcarpet::Render::HTML
    def block_code(code, language)
      Pygments.highlight(code, lexer: language)
    end
  end

  MARKDOWN = Redcarpet::Markdown.new(PygmentsMarkdownRenderer, fenced_code_blocks: true)
end
