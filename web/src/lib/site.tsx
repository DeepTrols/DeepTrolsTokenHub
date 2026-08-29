import React, { createContext, useContext, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";

export interface SiteInfo {
  site_name: string;
  logo_url: string;
  favicon_url: string;
  footer_text: string;
  notice: string;
  about: string;
  home_page_content: string;
  server_address: string;
  contact_email: string;
  legal: { user_agreement: string; privacy_policy: string };
  oauth_providers: string[];
}

function defaultSiteInfo(): SiteInfo {
  return {
    site_name: "DeepTrols",
    logo_url: "",
    favicon_url: "",
    footer_text: "",
    notice: "",
    about: "",
    home_page_content: "",
    server_address: "",
    contact_email: "",
    legal: { user_agreement: "", privacy_policy: "" },
    oauth_providers: [],
  };
}

export async function fetchSiteInfo(): Promise<SiteInfo> {
  const res = await fetch("/api/public/site", { credentials: "include" });
  if (!res.ok) {
    return defaultSiteInfo();
  }
  try {
    return (await res.json()) as SiteInfo;
  } catch {
    return defaultSiteInfo();
  }
}

interface SiteContextValue {
  site: SiteInfo;
  isLoading: boolean;
}

const SiteContext = createContext<SiteContextValue>({
  site: defaultSiteInfo(),
  isLoading: true,
});

export function SiteProvider({ children }: { children: React.ReactNode }) {
  const { data, isLoading } = useQuery({
    queryKey: ["site"],
    queryFn: fetchSiteInfo,
    staleTime: 5 * 60_000,
  });
  const site = data ?? defaultSiteInfo();

  useEffect(() => {
    document.title = site.site_name ? `${site.site_name} - AI Token Platform` : "DeepTrols - AI Token Platform";
    if (site.favicon_url) {
      let link = document.querySelector<HTMLLinkElement>("link[rel~='icon']");
      if (!link) {
        link = document.createElement("link");
        link.rel = "icon";
        document.head.appendChild(link);
      }
      link.href = site.favicon_url;
    }
  }, [site.site_name, site.favicon_url]);

  return <SiteContext.Provider value={{ site, isLoading }}>{children}</SiteContext.Provider>;
}

export const useSiteInfo = () => useContext(SiteContext);
