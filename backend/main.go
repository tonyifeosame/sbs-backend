package main
import ("fmt"; "github.com/gin-gonic/gin"; "github.com/golang-jwt/jwt/v5")
var secret = []byte("surebetslips2025")
var wins, losses int = 42, 8
type Bet struct { Games string `json:"games"`; Result bool `json:"result"` }
func main() {
  r := gin.Default(); r.Use(func(c *gin.Context){c.Header("Access-Control-Allow-Origin","*");c.Next()})
  r.POST("/login", func(c *gin.Context){token:=jwt.NewWithClaims(jwt.SigningMethodHS256,jwt.MapClaims{"user":"tony"});t,_:=token.SignedString(secret);c.JSON(200,gin.H{"token":t,"wins":wins,"losses":losses})})
  r.GET("/profile",auth,func(c *gin.Context){c.JSON(200,gin.H{"wins":wins,"losses":losses})})
  r.POST("/betslip",auth,func(c *gin.Context){
    var b Bet; c.BindJSON(&b)
    if b.Result { wins++ } else { losses++ }
    c.JSON(200,gin.H{"message":"Bet recorded!","wins":wins,"losses":losses})
  })
  r.Run(":8080")
}
func auth(c *gin.Context){
  t:=c.GetHeader("Authorization")
  token,_:=jwt.Parse(t,func(*jwt.Token)(any,error){return secret,nil})
  if token.Valid{c.Next()}else{c.AbortWithStatus(401)}
}
