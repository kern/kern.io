import "./globals.css";
import { Outfit } from "next/font/google";
import { ThemeProvider } from "@/components/theme-provider";
import { Analytics } from "@/components/analytics";
import { ModeToggle } from "@/components/mode-toggle";
import { PageNav } from "@/components/nav";
import Image from "next/image";
import Link from "next/link";
import { ExternalLink } from "@/components/external-link";
import { Metadata } from "next";

const outfit = Outfit({ subsets: ["latin"] });

export const metadata: Metadata = {
  title: "alex kern • kern.io",
  description: "Hacker, Designer, Founder",
  authors: {
    name: "Alex Kern",
    url: "https://kern.io",
  },
  viewport: "width=device-width, initial-scale=1",
  metadataBase: new URL("https://kern.io"),
  openGraph: {
    type: "website",
    url: "https://kern.io",
    title: "alex kern • kern.io",
    description: "Hacker, Designer, Founder",
    siteName: "Kern.io",
    images: [
      {
        url: "https://kern.io/facebook.png",
        width: 1200,
        height: 1200,
      },
    ],
  },
  twitter: {
    card: "summary_large_image",
    title: "alex kern • kern.io",
    description: "Hacker, Designer, Founder",
    creator: "@kernio",
    creatorId: "18856327",
    images: ["https://kern.io/facebook.png"],
  },
};

interface RootLayoutProps {
  children: React.ReactNode;
}

export default function RootLayout({ children }: RootLayoutProps) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body
        className={`antialiased min-h-screen bg-white text-stone-900 dark:text-stone-50 dark:bg-stone-950 ${outfit.className}`}
      >
        <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
          <div className="max-w-2xl mx-auto py-6 md:py-10 px-4">
            <PageNav />
            <main>{children}</main>
            <footer className="mx-auto mt-12 mb-4 flex items-center justify-center text-xs">
              <div>
                Released under the{" "}
                <ExternalLink href="https://github.com/kern/kern.io/blob/master/LICENSE">
                  BSD 3-Clause license
                </ExternalLink>{" "}
                &middot;{" "}
                <ExternalLink href="https://github.com/kern/kern.io">
                  Fork me
                </ExternalLink>
              </div>

              <div className="pl-2">
                <ModeToggle />
              </div>
            </footer>
          </div>
          <Analytics />
        </ThemeProvider>
      </body>
    </html>
  );
}
