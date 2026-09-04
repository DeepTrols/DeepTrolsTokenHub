import { Toaster as Sonner } from "sonner";

type ToasterProps = React.ComponentProps<typeof Sonner>;

const Toaster = ({ ...props }: ToasterProps) => (
  <Sonner
    className="toaster group"
    toastOptions={{
      classNames: {
        toast: "group toast group-[.toaster]:glass group-[.toaster]:text-foreground group-[.toaster]:border-white/80 group-[.toaster]:rounded-2xl group-[.toaster]:shadow-[0_22px_60px_rgba(137,76,32,0.12)]",
        description: "group-[.toast]:text-muted-foreground",
        actionButton: "group-[.toast]:bg-gradient-to-br group-[.toast]:from-[#F78B28] group-[.toast]:to-[#E85D3F] group-[.toast]:text-white",
        cancelButton: "group-[.toast]:bg-muted group-[.toast]:text-muted-foreground",
      },
    }}
    {...props}
  />
);

export { Toaster };
