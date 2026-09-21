package application

import "github.com/gin-gonic/gin"

type Port interface{ Run(*gin.Context) }
