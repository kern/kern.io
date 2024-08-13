import { getAllPosts } from '@/lib/api';

export default async function sitemap() {
  const baseUrl = 'https://kern.io';
  const posts = await getAllPosts();

  const postUrls = posts.map((post) => ({
    url: `${baseUrl}/blog/${post.slug}`,
    lastModified: new Date(post.date).toISOString(),
  }));

  return [
    {
      url: baseUrl,
      lastModified: new Date().toISOString(),
    },
    ...postUrls,
  ];
}
