import { RouterView, type RouteRecordRaw } from "vue-router"

/** 管理域路由独占 /admin 及所有后代，未来子页只修改本模块。 */
export const adminRoutes: readonly RouteRecordRaw[] = [
  {
    path: "/admin",
    component: RouterView,
    children: [
      {
        path: "",
        name: "admin-unavailable",
        component: () => import("@/views/admin/AdminUnavailableView.vue"),
        meta: { title: "管理后台暂不可用" },
      },
      {
        path: ":pathMatch(.*)*",
        name: "admin-unavailable-child",
        component: () => import("@/views/admin/AdminUnavailableView.vue"),
        meta: { title: "管理后台暂不可用" },
      },
    ],
  },
]
