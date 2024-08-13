import fs from 'fs';
import path from 'path';

export default function handler(req, res) {
  const sitemapPath = path.join(process.cwd(), 'public', 'sitemap.xml');

  try {
    const sitemapContent = fs.readFileSync(sitemapPath, 'utf8');
    res.setHeader('Content-Type', 'application/xml');
    res.status(200).send(sitemapContent);
  } catch (error) {
    console.error('Error reading sitemap.xml:', error);
    res.status(500).json({ error: 'Unable to read sitemap' });
  }
}
