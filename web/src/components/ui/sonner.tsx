import { Toaster as Sonner } from "sonner";

type ToasterProps = React.ComponentProps<typeof Sonner>;

const Toaster = ({ ...props }: ToasterProps) => (
  <Sonner
    className="toaster group"
    toastOptions={{
      classNames: {
        toast: "group toast group-[.toaster]:glass group-[.toaster]:text-foreground group-[.toaster]:border-white/80 group-[.toaster]:rounded-2xl group-[.toaster]:shadow-[0_22px_60px_rgba(63,76,128,0.12)]",
        description: "group-[.toast]:text-muted-foreground",
        actionButton: "group-[.toast]:bg-gradient-to-br group-[.toast]:from-[#4F6BED] group-[.toast]:to-[#8B6FE8] group-[.toast]:text-white",
        cancelButton: "group-[.toast]:bg-muted group-[.toast]:text-muted-foreground",
      },
    }}
    {...props}
  />
);

export { Toaster };
