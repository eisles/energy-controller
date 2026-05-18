import "./globals.css";

export const metadata = {
  title: "Energy Controller",
  description: "Mock simulation dashboard for home energy control"
};

export default function RootLayout({
  children
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="ja">
      <body>{children}</body>
    </html>
  );
}
