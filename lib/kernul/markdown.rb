module Kernul
  class Markdown
    def initialize(text)
      @renderer = Kramdown::Document.new(text,
                                         :coderay_css => :class,
                                         :coderay_line_numbers => nil)
    end

    def to_html
      @renderer.to_html
    end
  end
end
